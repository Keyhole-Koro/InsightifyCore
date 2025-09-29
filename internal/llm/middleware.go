package llm

import (
    "context"
    "encoding/json"
    "log"
    "os"
    "strconv"
    "time"
)

// Middleware decorates an LLMClient to inject cross-cutting concerns
// (rate limiting, retries, logging, hooks, etc.).
type Middleware func(LLMClient) LLMClient

// Wrap applies middlewares in left-to-right order.
// Example: Wrap(inner, A, B) => A(B(inner))
func Wrap(inner LLMClient, mws ...Middleware) LLMClient {
	out := inner
	for i := len(mws) - 1; i >= 0; i-- {
		out = mws[i](out)
	}
	return out
}

// -------- Rate Limiting (using existing rpsLimiter) --------

// RateLimit limits request rate using the custom rpsLimiter.
// If rps <= 0, the limiter is effectively disabled.
func RateLimit(rps float64, burst int) Middleware {
	return func(next LLMClient) LLMClient {
		rl := newRPSLimiter(rps, burst) // nil when disabled
		return &rateLimited{next: next, rl: rl}
	}
}

type rateLimited struct {
	next LLMClient
	rl   *rpsLimiter
}

func (c *rateLimited) Name() string { return c.next.Name() }
func (c *rateLimited) Close() error { return c.next.Close() }
func (c *rateLimited) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	if c.rl != nil {
		// Prefer reserved credits embedded in the context.
		if !TakeCredit(ctx) {
			if err := c.rl.Acquire(ctx); err != nil {
				return nil, err
			}
		}
	}
	return c.next.GenerateJSON(ctx, prompt, input)
}

// RateLimitFromEnv reads RPS/BURST from environment variables with the
// given prefixes in priority order. For example, ("LLM","GEMINI")
// checks LLM_RPS/LLM_BURST first, then GEMINI_RPS/GEMINI_BURST.
func RateLimitFromEnv(prefixes ...string) Middleware {
	readFloat := func(key string) float64 {
		if key == "" {
			return 0
		}
		v := os.Getenv(key)
		if v == "" {
			return 0
		}
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	readInt := func(key string) int {
		if key == "" {
			return 0
		}
		v := os.Getenv(key)
		if v == "" {
			return 0
		}
		n, _ := strconv.Atoi(v)
		return n
	}
	find := func(suffix string) string {
		for _, p := range prefixes {
			if p == "" {
				continue
			}
			k := p + suffix
			if os.Getenv(k) != "" {
				return k
			}
		}
		return ""
	}
	return func(next LLMClient) LLMClient {
		rps := readFloat(find("_RPS"))
		burst := readInt(find("_BURST"))
		rl := newRPSLimiter(rps, burst) // nil when disabled
		return &rateLimited{next: next, rl: rl}
	}
}

// -------- RPM/RPD/TPM combined limiter --------

// MultiLimit applies minute/day request limits and tokens-per-minute.
// Pass 0 to disable a specific limiter. Burst is derived as the nominal rate.
func MultiLimit(rpm, rpd, tpm int) Middleware {
    // Default to a constant token-per-request estimate to avoid per-call
    // marshaling and string work. Adjust via MultiLimitConstTokens if needed.
    const defaultTokensPerRequest = 1000
    return MultiLimitConstTokens(rpm, rpd, tpm, defaultTokensPerRequest)
}

// MultiLimitConstTokens is like MultiLimit but uses a constant tokens-per-request
// estimate for the TPM limiter.
func MultiLimitConstTokens(rpm, rpd, tpm int, tokensPerRequest int) Middleware {
    var rpmL, rpdL, tpmL *rpsLimiter
    if rpm > 0 {
        rpmL = newRPSLimiter(float64(rpm)/60.0, max1(rpm))
    }
    if rpd > 0 {
        rpdL = newRPSLimiter(float64(rpd)/86400.0, max1(rpd))
    }
    if tpm > 0 {
        tpmL = newRPSLimiter(float64(tpm)/60.0, max1(tpm))
    }
    if tokensPerRequest < 1 { tokensPerRequest = 1 }
    return func(next LLMClient) LLMClient {
        return &multiLimited{next: next, rpm: rpmL, rpd: rpdL, tpm: tpmL, tpr: tokensPerRequest}
    }
}

// MultiLimitFromEnv reads _RPM, _RPD, _TPM (ints) using prefixes priority.
func MultiLimitFromEnv(prefixes ...string) Middleware {
	readInt := func(key string) int {
		if key == "" {
			return 0
		}
		v := os.Getenv(key)
		if v == "" {
			return 0
		}
		n, _ := strconv.Atoi(v)
		return n
	}
	find := func(suffix string) string {
		for _, p := range prefixes {
			if p == "" {
				continue
			}
			k := p + suffix
			if os.Getenv(k) != "" {
				return k
			}
		}
		return ""
	}
    rpm := readInt(find("_RPM"))
    rpd := readInt(find("_RPD"))
    tpm := readInt(find("_TPM"))
    return MultiLimit(rpm, rpd, tpm)
}

type multiLimited struct {
    next LLMClient
    rpm  *rpsLimiter
    rpd  *rpsLimiter
    tpm  *rpsLimiter
    tpr  int // tokens per request (constant estimate)
}

func (m *multiLimited) Name() string { return m.next.Name() }
func (m *multiLimited) Close() error { return m.next.Close() }
func (m *multiLimited) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	// Requests-per-minute/day
	if m.rpm != nil {
		if !TakeCredit(ctx) {
			if err := m.rpm.Acquire(ctx); err != nil {
				return nil, err
			}
		}
	}
	if m.rpd != nil {
		if !TakeCredit(ctx) {
			if err := m.rpd.Acquire(ctx); err != nil {
				return nil, err
			}
		}
	}
    // Tokens-per-minute using a constant per-request estimate.
    if m.tpm != nil {
        est := m.tpr
        if est < 1 { est = 1 }
        if err := m.tpm.AcquireN(ctx, est); err != nil {
            return nil, err
        }
    }
    return m.next.GenerateJSON(ctx, prompt, input)
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// -------- Retry with exponential backoff --------

// Retry retries GenerateJSON up to maxAttempts with exponential backoff
// starting at baseDelay. If context is canceled, it stops immediately.
func Retry(maxAttempts int, baseDelay time.Duration) Middleware {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if baseDelay <= 0 {
		baseDelay = 300 * time.Millisecond
	}
	return func(next LLMClient) LLMClient {
		return &retrying{next: next, max: maxAttempts, base: baseDelay}
	}
}

type retrying struct {
	next LLMClient
	max  int
	base time.Duration
}

func (r *retrying) Name() string { return r.next.Name() }
func (r *retrying) Close() error { return r.next.Close() }
func (r *retrying) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	var last error
	for i := 0; i < r.max; i++ {
		resp, err := r.next.GenerateJSON(ctx, prompt, input)
		if err == nil {
			return resp, nil
		}
		last = err
		// Stop immediately if the context is canceled.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		time.Sleep(r.base * time.Duration(1<<i))
	}
	return nil, last
}

// -------- Logging & Hooks --------

// WithLogging logs request size and errors. Provide a custom logger or nil
// to use log.Default().
func WithLogging(logger *log.Logger) Middleware {
	if logger == nil {
		logger = log.Default()
	}
	return func(next LLMClient) LLMClient {
		return &logging{next: next, log: logger}
	}
}

type logging struct {
	next LLMClient
	log  *log.Logger
}

func (l *logging) Name() string { return l.next.Name() }
func (l *logging) Close() error { return l.next.Close() }
func (l *logging) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	in, _ := json.MarshalIndent(input, "", "  ")
	l.log.Printf("LLM request (%s): %d bytes", PhaseFrom(ctx), len(prompt)+len(in))
	raw, err := l.next.GenerateJSON(ctx, prompt, input)
	if err != nil {
		l.log.Printf("LLM error (%s): %v", PhaseFrom(ctx), err)
	}
	return raw, err
}

// WithHooks calls HookFrom(ctx).Before/After around GenerateJSON.
// If no hook is present in the context, it is a no-op.
func WithHooks() Middleware {
	return func(next LLMClient) LLMClient {
		return &hooked{next: next}
	}
}

type hooked struct{ next LLMClient }

func (h *hooked) Name() string { return h.next.Name() }
func (h *hooked) Close() error { return h.next.Close() }
func (h *hooked) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	if hook := HookFrom(ctx); hook != nil {
		hook.Before(ctx, PhaseFrom(ctx), prompt, input)
	}
	raw, err := h.next.GenerateJSON(ctx, prompt, input)
	if hook := HookFrom(ctx); hook != nil {
		hook.After(ctx, PhaseFrom(ctx), raw, err)
	}
	return raw, err
}
