package llm

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	llmclient "insightify/internal/llmClient"
)

type passthroughClient struct{}

func (p *passthroughClient) Name() string { return "pass" }
func (p *passthroughClient) Close() error { return nil }
func (p *passthroughClient) CountTokens(text string) int {
	_ = text
	return 1
}
func (p *passthroughClient) TokenCapacity() int { return 1 }
func (p *passthroughClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	_ = ctx
	_ = prompt
	_ = input
	return json.RawMessage(`{"ok":true}`), nil
}
func (p *passthroughClient) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	return p.GenerateJSON(ctx, prompt, input)
}

type awareWaitClient struct {
	passthroughClient
	headers llmclient.RateLimitHeaders
	has     bool
}

type fixedWaitAdapter struct {
	wait time.Duration
}

func (a fixedWaitAdapter) NextWait(headers llmclient.RateLimitHeaders) time.Duration {
	_ = headers
	return a.wait
}

func (a *awareWaitClient) SetRateLimitHeaderHandler(handler llmclient.RateLimitHeaderHandler) {
	_ = handler
}
func (a *awareWaitClient) LastRateLimitHeaders() (llmclient.RateLimitHeaders, bool) {
	return a.headers, a.has
}

func TestRespectRateLimitSignals_WaitsByAdapter(t *testing.T) {
	inner := &passthroughClient{}
	cli := Wrap(inner, RespectRateLimitSignals(fixedWaitAdapter{wait: 25 * time.Millisecond}))

	sel := &awareWaitClient{headers: llmclient.RateLimitHeaders{RetryAfterSeconds: 1}, has: true}
	ctx := withSelectedModel(context.Background(), selectedModel{client: sel})

	start := time.Now()
	if _, err := cli.GenerateJSON(ctx, "p", nil); err != nil {
		t.Fatalf("generate: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 20*time.Millisecond {
		t.Fatalf("expected middleware wait, got=%s", elapsed)
	}
}
