package llmclient

import (
	"net/http"
	"testing"
	"time"
)

func TestParseGroqRateLimitHeaders_GroqFormat(t *testing.T) {
	h := http.Header{}
	h.Set("retry-after", "2")
	h.Set("x-ratelimit-limit-requests", "14400")
	h.Set("x-ratelimit-limit-tokens", "18000")
	h.Set("x-ratelimit-remaining-requests", "14370")
	h.Set("x-ratelimit-remaining-tokens", "17997")
	h.Set("x-ratelimit-reset-requests", "2m59.56s")
	h.Set("x-ratelimit-reset-tokens", "7.66s")

	got, ok := parseGroqRateLimitHeaders(h)
	if !ok {
		t.Fatalf("expected headers to be parsed")
	}
	if got.RetryAfterSeconds != 2 {
		t.Fatalf("retry-after: got=%d", got.RetryAfterSeconds)
	}
	if got.LimitRequests != 14400 || got.LimitTokens != 18000 {
		t.Fatalf("limits: got requests=%d tokens=%d", got.LimitRequests, got.LimitTokens)
	}
	if got.RemainingRequests != 14370 || got.RemainingTokens != 17997 {
		t.Fatalf("remaining: got requests=%d tokens=%d", got.RemainingRequests, got.RemainingTokens)
	}
	if got.ResetRequests != (2*time.Minute + 59*time.Second + 560*time.Millisecond) {
		t.Fatalf("reset requests: got=%s", got.ResetRequests)
	}
	if got.ResetTokens != (7*time.Second + 660*time.Millisecond) {
		t.Fatalf("reset tokens: got=%s", got.ResetTokens)
	}
}

func TestHeaderRateLimitControlAdapter_NextWait(t *testing.T) {
	adapter := HeaderRateLimitControlAdapter{}
	if got := adapter.NextWait(RateLimitHeaders{RetryAfterSeconds: 3}); got != 3*time.Second {
		t.Fatalf("retry-after wait: got=%s", got)
	}
	if got := adapter.NextWait(RateLimitHeaders{RemainingTokens: 0, ResetTokens: 5 * time.Second}); got != 5*time.Second {
		t.Fatalf("token reset wait: got=%s", got)
	}
	if got := adapter.NextWait(RateLimitHeaders{RemainingRequests: 0, ResetRequests: 11 * time.Second}); got != 11*time.Second {
		t.Fatalf("request reset wait: got=%s", got)
	}
	if got := adapter.NextWait(RateLimitHeaders{RemainingTokens: 10}); got != 0 {
		t.Fatalf("no wait expected: got=%s", got)
	}
}
