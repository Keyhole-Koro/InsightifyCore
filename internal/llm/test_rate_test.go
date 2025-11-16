package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	llmclient "insightify/internal/llmClient"
	"insightify/internal/tester"
)

// fast fake client that returns immediately
type fastClient struct{}

func (f *fastClient) Name() string                { return "fast" }
func (f *fastClient) Close() error                { return nil }
func (f *fastClient) CountTokens(text string) int { return len(strings.Fields(text)) }
func (f *fastClient) TokenCapacity() int          { return 1024 }
func (f *fastClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	return json.RawMessage([]byte(`{}`)), nil
}

// spy records timestamps when requests reach the inner client
type spy struct{ times []time.Time }
type spyingClient struct {
	next llmclient.LLMClient
	rec  *spy
}

func (s *spyingClient) Name() string { return s.next.Name() }
func (s *spyingClient) Close() error { return s.next.Close() }
func (s *spyingClient) CountTokens(text string) int {
	return s.next.CountTokens(text)
}
func (s *spyingClient) TokenCapacity() int { return s.next.TokenCapacity() }
func (s *spyingClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	s.rec.times = append(s.rec.times, time.Now())
	return s.next.GenerateJSON(ctx, prompt, input)
}

func TestRate_RPS_2PerSecond_Burst1_Spacing(t *testing.T) {
	// Expect ~>=500ms spacing after the first call when rps=2 and burst=1.
	base := &fastClient{}
	rec := &spy{}
	cli := Wrap(&spyingClient{next: base, rec: rec}, RateLimit(2, 1))
	t.Cleanup(func() { _ = cli.Close() })

	ctx := context.Background()
	start := time.Now()
	// Two sequential calls; first should pass immediately, second should wait ~500ms.
	if _, err := cli.GenerateJSON(ctx, "p", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err := cli.GenerateJSON(ctx, "p", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	tester.True(t, elapsed >= 450*time.Millisecond, "expected throttling >=450ms, got %v", elapsed)
	tester.Eq(t, len(rec.times), 2, "two calls should reach inner client")
}

func TestRate_RPS_2PerSecond_Burst2_FirstTwoImmediate(t *testing.T) {
	// With burst=2, first two calls should be near-instant; third should be delayed.
	base := &fastClient{}
	cli := RateLimit(2, 2)(base)
	t.Cleanup(func() { _ = cli.Close() })

	ctx := context.Background()
	// First two calls back-to-back
	start := time.Now()
	if _, err := cli.GenerateJSON(ctx, "p", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err := cli.GenerateJSON(ctx, "p", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	firstTwo := time.Since(start)

	// Third call should incur ~>=500ms delay at 2 rps.
	start3 := time.Now()
	if _, err := cli.GenerateJSON(ctx, "p", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	third := time.Since(start3)

	tester.True(t, firstTwo < 100*time.Millisecond, "first two should be near-instant, got %v", firstTwo)
	tester.True(t, third >= 450*time.Millisecond, "third call expected throttling >=450ms, got %v", third)
}
