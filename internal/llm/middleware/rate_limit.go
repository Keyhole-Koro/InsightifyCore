package llm

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	llmclient "insightify/internal/llm/client"
)

// ----------------------------------------------------------------------------
// rpsLimiter – lightweight token-bucket limiter
// ----------------------------------------------------------------------------

// rpsLimiter is a lightweight token-bucket limiter that throttles to at most
// R requests per second with an optional burst capacity.
type rpsLimiter struct {
	tokens chan struct{}
	stopCh chan struct{}
}

// newRPSLimiter creates a limiter that allows up to rps events per second
// with a burst capacity of 'burst'. If rps <= 0, the limiter is disabled
// (Acquire becomes a no-op).
func newRPSLimiter(rps float64, burst int) *rpsLimiter {
	if rps <= 0 {
		return nil
	}
	if burst <= 0 {
		burst = 1
	}

	l := &rpsLimiter{
		tokens: make(chan struct{}, burst),
		stopCh: make(chan struct{}),
	}

	// Pre-fill bucket to allow an initial burst.
	for i := 0; i < burst; i++ {
		l.tokens <- struct{}{}
	}

	// Refill at the configured rate.
	period := time.Duration(float64(time.Second) / rps)
	if period <= 0 {
		period = time.Millisecond // safeguard
	}
	ticker := time.NewTicker(period)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				select {
				case l.tokens <- struct{}{}:
				default:
					// bucket full; drop token
				}
			case <-l.stopCh:
				return
			}
		}
	}()

	return l
}

// Acquire blocks until a token is available or the context is canceled.
func (l *rpsLimiter) Acquire(ctx context.Context) error {
	if l == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.stopCh:
		return context.Canceled
	case <-l.tokens:
		return nil
	}
}

// AcquireN acquires n tokens sequentially.
func (l *rpsLimiter) AcquireN(ctx context.Context, n int) error {
	if l == nil || n <= 0 {
		return nil
	}
	for i := 0; i < n; i++ {
		if err := l.Acquire(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Stop terminates the limiter's refill goroutine.
func (l *rpsLimiter) Stop() {
	if l == nil {
		return
	}
	close(l.stopCh)
}

// Limiter is a minimal interface for an existing token/rps limiter.
type Limiter interface {
	Acquire(ctx context.Context) error
}

// NewLimiter exposes a minimal Limiter backed by an internal rpsLimiter.
func NewLimiter(rps float64, burst int) Limiter {
	return newRPSLimiter(rps, burst)
}

// ----------------------------------------------------------------------------
// PermitBroker – reserve permits up-front
// ----------------------------------------------------------------------------

// PermitBroker reserves N permits up-front.
type PermitBroker interface {
	Reserve(ctx context.Context, n int) (Lease, error)
}

// Lease injects reserved credits into a context.
type Lease interface {
	Context(ctx context.Context) context.Context
}

type broker struct{ rl Limiter }

// NewBroker returns a PermitBroker backed by the given limiter.
func NewBroker(rl Limiter) PermitBroker { return &broker{rl: rl} }

// Reserve acquires n permits from the limiter and returns a lease.
func (b *broker) Reserve(ctx context.Context, n int) (Lease, error) {
	if n <= 0 || b == nil || b.rl == nil {
		return lease{n: 0}, nil
	}
	for i := 0; i < n; i++ {
		if err := b.rl.Acquire(ctx); err != nil {
			return nil, err
		}
	}
	return lease{n: n}, nil
}

type lease struct{ n int }

// Context injects reserved credits into the provided context.
func (l lease) Context(ctx context.Context) context.Context { return WithCredits(ctx, l.n) }

// ----------------------------------------------------------------------------
// Credits – atomic counter for reserved credits in context
// ----------------------------------------------------------------------------

type creditsKey struct{}
type credits struct{ n int32 }

// WithCredits returns a context that carries n consumable credits.
func WithCredits(ctx context.Context, n int) context.Context {
	if n <= 0 {
		return ctx
	}
	c := &credits{n: int32(n)}
	return context.WithValue(ctx, creditsKey{}, c)
}

// TakeCredit atomically consumes one credit from the context if available.
func TakeCredit(ctx context.Context) bool {
	v := ctx.Value(creditsKey{})
	if v == nil {
		return false
	}
	c, ok := v.(*credits)
	if !ok || c == nil {
		return false
	}
	for {
		cur := atomic.LoadInt32(&c.n)
		if cur <= 0 {
			return false
		}
		if atomic.CompareAndSwapInt32(&c.n, cur, cur-1) {
			return true
		}
	}
}

// ----------------------------------------------------------------------------
// RateLimit middleware
// ----------------------------------------------------------------------------

// RateLimit limits request rate using the custom rpsLimiter.
func RateLimit(rps float64, burst int) Middleware {
	return func(next llmclient.LLMClient) llmclient.LLMClient {
		rl := newRPSLimiter(rps, burst)
		return &rateLimited{next: next, rl: rl}
	}
}

type rateLimited struct {
	next llmclient.LLMClient
	rl   *rpsLimiter
}

func (c *rateLimited) Name() string { return c.next.Name() }
func (c *rateLimited) Close() error { return c.next.Close() }
func (c *rateLimited) CountTokens(text string) int {
	return c.next.CountTokens(text)
}
func (c *rateLimited) TokenCapacity() int { return c.next.TokenCapacity() }

func (c *rateLimited) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	if c.rl != nil {
		if !TakeCredit(ctx) {
			if err := c.rl.Acquire(ctx); err != nil {
				return nil, err
			}
		}
	}
	return c.next.GenerateJSON(ctx, prompt, input)
}

func (c *rateLimited) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	if c.rl != nil {
		if !TakeCredit(ctx) {
			if err := c.rl.Acquire(ctx); err != nil {
				return nil, err
			}
		}
	}
	return c.next.GenerateJSONStream(ctx, prompt, input, onChunk)
}

// RateLimitFromEnv reads RPS/BURST from environment variables.
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
	return func(next llmclient.LLMClient) llmclient.LLMClient {
		rps := readFloat(find("_RPS"))
		burst := readInt(find("_BURST"))
		rl := newRPSLimiter(rps, burst)
		return &rateLimited{next: next, rl: rl}
	}
}

// ----------------------------------------------------------------------------
// MultiLimit middleware – RPM/RPD/TPM combined limiter
// ----------------------------------------------------------------------------

// MultiLimit applies minute/day request limits and tokens-per-minute.
func MultiLimit(rpm, rpd, tpm int) Middleware {
	const defaultTokensPerRequest = 1000
	return MultiLimitConstTokens(rpm, rpd, tpm, defaultTokensPerRequest)
}

// MultiLimitConstTokens is like MultiLimit but uses a constant tokens-per-request estimate.
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
	if tokensPerRequest < 1 {
		tokensPerRequest = 1
	}
	return func(next llmclient.LLMClient) llmclient.LLMClient {
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

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

type multiLimited struct {
	next llmclient.LLMClient
	rpm  *rpsLimiter
	rpd  *rpsLimiter
	tpm  *rpsLimiter
	tpr  int
}

func (m *multiLimited) Name() string { return m.next.Name() }
func (m *multiLimited) Close() error { return m.next.Close() }
func (m *multiLimited) CountTokens(text string) int {
	return m.next.CountTokens(text)
}
func (m *multiLimited) TokenCapacity() int { return m.next.TokenCapacity() }

func (m *multiLimited) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
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
	if m.tpm != nil {
		est := m.tpr
		if est < 1 {
			est = 1
		}
		if err := m.tpm.AcquireN(ctx, est); err != nil {
			return nil, err
		}
	}
	return m.next.GenerateJSON(ctx, prompt, input)
}

func (m *multiLimited) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
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
	if m.tpm != nil {
		est := m.tpr
		if est < 1 {
			est = 1
		}
		if err := m.tpm.AcquireN(ctx, est); err != nil {
			return nil, err
		}
	}
	return m.next.GenerateJSONStream(ctx, prompt, input, onChunk)
}

// ----------------------------------------------------------------------------
// TokenDayLimit middleware
// ----------------------------------------------------------------------------

// TokenDayLimit limits approximate tokens per day.
func TokenDayLimit(tpd int) Middleware {
	const defaultTokensPerRequest = 1000
	return TokenDayLimitConstTokens(tpd, defaultTokensPerRequest)
}

// TokenDayLimitConstTokens is like TokenDayLimit but allows a custom estimate.
func TokenDayLimitConstTokens(tpd int, tokensPerRequest int) Middleware {
	var tpdL *rpsLimiter
	if tpd > 0 {
		tpdL = newRPSLimiter(float64(tpd)/86400.0, max1(tpd))
	}
	if tokensPerRequest < 1 {
		tokensPerRequest = 1
	}
	return func(next llmclient.LLMClient) llmclient.LLMClient {
		return &tokenDayLimited{next: next, tpd: tpdL, tpr: tokensPerRequest}
	}
}

type tokenDayLimited struct {
	next llmclient.LLMClient
	tpd  *rpsLimiter
	tpr  int
}

func (m *tokenDayLimited) Name() string { return m.next.Name() }
func (m *tokenDayLimited) Close() error { return m.next.Close() }
func (m *tokenDayLimited) CountTokens(text string) int {
	return m.next.CountTokens(text)
}
func (m *tokenDayLimited) TokenCapacity() int { return m.next.TokenCapacity() }

func (m *tokenDayLimited) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	if m.tpd != nil {
		est := m.tpr
		if est < 1 {
			est = 1
		}
		if err := m.tpd.AcquireN(ctx, est); err != nil {
			return nil, err
		}
	}
	return m.next.GenerateJSON(ctx, prompt, input)
}

func (m *tokenDayLimited) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	if m.tpd != nil {
		est := m.tpr
		if est < 1 {
			est = 1
		}
		if err := m.tpd.AcquireN(ctx, est); err != nil {
			return nil, err
		}
	}
	return m.next.GenerateJSONStream(ctx, prompt, input, onChunk)
}

// ----------------------------------------------------------------------------
// RespectRateLimitSignals middleware
// ----------------------------------------------------------------------------

// RespectRateLimitSignals delays requests based on the selected model's last
// observed provider rate-limit signals.
func RespectRateLimitSignals(adapter llmclient.RateLimitControlAdapter) Middleware {
	if adapter == nil {
		adapter = llmclient.HeaderRateLimitControlAdapter{}
	}
	return func(next llmclient.LLMClient) llmclient.LLMClient {
		return &rateLimitSignalControlled{next: next, adapter: adapter}
	}
}

type rateLimitSignalControlled struct {
	next    llmclient.LLMClient
	adapter llmclient.RateLimitControlAdapter
}

func (m *rateLimitSignalControlled) Name() string { return m.next.Name() }
func (m *rateLimitSignalControlled) Close() error { return m.next.Close() }
func (m *rateLimitSignalControlled) CountTokens(text string) int {
	return m.next.CountTokens(text)
}
func (m *rateLimitSignalControlled) TokenCapacity() int { return m.next.TokenCapacity() }

func (m *rateLimitSignalControlled) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	if err := m.wait(ctx); err != nil {
		return nil, err
	}
	return m.next.GenerateJSON(ctx, prompt, input)
}

func (m *rateLimitSignalControlled) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	if err := m.wait(ctx); err != nil {
		return nil, err
	}
	return m.next.GenerateJSONStream(ctx, prompt, input, onChunk)
}

func (m *rateLimitSignalControlled) wait(ctx context.Context) error {
	if m.adapter == nil {
		return nil
	}
	selected, ok := SelectedClientFrom(ctx)
	if !ok || selected == nil {
		return nil
	}
	aware, ok := selected.(llmclient.RateLimitHeaderAwareClient)
	if !ok {
		return nil
	}
	headers, ok := aware.LastRateLimitHeaders()
	if !ok {
		return nil
	}
	wait := m.adapter.NextWait(headers)
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
