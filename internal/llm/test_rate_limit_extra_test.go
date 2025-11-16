package llm

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// fake client that returns empty JSON quickly
type tinyClient struct{}

func (t *tinyClient) Name() string                { return "tiny" }
func (t *tinyClient) Close() error                { return nil }
func (t *tinyClient) CountTokens(text string) int { return len(text) }
func (t *tinyClient) TokenCapacity() int          { return 1024 }
func (t *tinyClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	return json.RawMessage([]byte(`{}`)), nil
}

func TestRateLimit_ThrottlesAt2RPS_Burst1(t *testing.T) {
	// Build a client limited to ~2 rps and burst 1.
	base := &tinyClient{}
	cli := RateLimit(2, 1)(base) // ~1 token every 500ms after initial

	// First call should pass immediately, second should be delayed ~>=500ms.
	ctx := context.Background()
	start := time.Now()
	if _, err := cli.GenerateJSON(ctx, "p", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err := cli.GenerateJSON(ctx, "p", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	// Allow slack for timers; expect at least ~450ms.
	if elapsed < 450*time.Millisecond {
		t.Fatalf("expected throttling >=450ms, got %v", elapsed)
	}
}

func TestLimiter_AcquireN_Works(t *testing.T) {
	// ~500 rps => period ~2ms. Acquire 10 tokens should complete well under 200ms.
	l := newRPSLimiter(500, 1)
	if l == nil {
		t.Fatal("limiter nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	if err := l.AcquireN(ctx, 10); err != nil {
		t.Fatalf("AcquireN: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Fatalf("AcquireN too slow: %v", elapsed)
	}
}
