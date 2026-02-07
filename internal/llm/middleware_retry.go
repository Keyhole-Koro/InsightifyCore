package llm

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	llmclient "insightify/internal/llmClient"
)

// Retry retries GenerateJSON up to maxAttempts with exponential backoff
// starting at baseDelay. If context is canceled, it stops immediately.
func Retry(maxAttempts int, baseDelay time.Duration) Middleware {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if baseDelay <= 0 {
		baseDelay = 300 * time.Millisecond
	}
	return func(next llmclient.LLMClient) llmclient.LLMClient {
		return &retrying{next: next, max: maxAttempts, base: baseDelay}
	}
}

type retrying struct {
	next llmclient.LLMClient
	max  int
	base time.Duration
}

func (r *retrying) Name() string { return r.next.Name() }
func (r *retrying) Close() error { return r.next.Close() }
func (r *retrying) CountTokens(text string) int {
	return r.next.CountTokens(text)
}
func (r *retrying) TokenCapacity() int { return r.next.TokenCapacity() }

func (r *retrying) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	var last error
	for i := 0; i < r.max; i++ {
		resp, err := r.next.GenerateJSON(ctx, prompt, input)
		if err == nil {
			return resp, nil
		}
		// If it's a permanent error, do not retry.
		var pErr *llmclient.PermanentError
		if errors.As(err, &pErr) {
			return nil, err
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

func (r *retrying) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	var last error
	for i := 0; i < r.max; i++ {
		resp, err := r.next.GenerateJSONStream(ctx, prompt, input, onChunk)
		if err == nil {
			return resp, nil
		}
		var pErr *llmclient.PermanentError
		if errors.As(err, &pErr) {
			return nil, err
		}
		last = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		time.Sleep(r.base * time.Duration(1<<i))
	}
	return nil, last
}
