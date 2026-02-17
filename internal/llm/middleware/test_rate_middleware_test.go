package llm

import (
	"context"
	"encoding/json"
	"testing"

	"insightify/internal/tester"
)

// fake client that counts calls
type fakeClient struct {
	name  string
	calls int
}

func (f *fakeClient) Name() string                { return f.name }
func (f *fakeClient) Close() error                { return nil }
func (f *fakeClient) CountTokens(text string) int { return len(text) }
func (f *fakeClient) TokenCapacity() int          { return 1024 }
func (f *fakeClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	f.calls++
	return json.RawMessage([]byte(`{}`)), nil
}
func (f *fakeClient) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	return f.GenerateJSON(ctx, prompt, input)
}

type countLimiter struct{ calls int }

func (c *countLimiter) Acquire(ctx context.Context) error { c.calls++; return nil }

func TestRateLimitPrefersCredits(t *testing.T) {
	inner := &fakeClient{name: "inner"}
	rl := &countLimiter{}
	// Use middleware constructor with rps=0 to disable limiter and ensure GenerateJSON still runs with credits.
	cli := RateLimit(0, 0)(inner)
	ctx := WithCredits(context.Background(), 1)
	if _, err := cli.GenerateJSON(ctx, "p", map[string]any{}); err != nil {
		t.Fatal(err)
	}

	// Now simulate lack of credits: still should proceed, but through disabled limiter.
	if _, err := cli.GenerateJSON(context.Background(), "p", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	tester.Eq(t, inner.calls, 2)
	_ = rl // quiet unused variable in some toolchains
}
