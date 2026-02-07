package llmclient

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// parseGroqRateLimitHeaders parses Groq-specific rate-limit response headers.
// Groq semantics:
// - request fields are RPD
// - token fields are TPM
func parseGroqRateLimitHeaders(h http.Header) (RateLimitHeaders, bool) {
	out := RateLimitHeaders{}
	found := false

	readInt := func(key string) (int, bool) {
		v := strings.TrimSpace(h.Get(key))
		if v == "" {
			return 0, false
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	readDur := func(key string) (time.Duration, bool) {
		v := strings.TrimSpace(h.Get(key))
		if v == "" {
			return 0, false
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0, false
		}
		return d, true
	}

	if v, ok := readInt("retry-after"); ok {
		out.RetryAfterSeconds = v
		found = true
	}
	if v, ok := readInt("x-ratelimit-limit-requests"); ok {
		out.LimitRequests = v
		found = true
	}
	if v, ok := readInt("x-ratelimit-limit-tokens"); ok {
		out.LimitTokens = v
		found = true
	}
	if v, ok := readInt("x-ratelimit-remaining-requests"); ok {
		out.RemainingRequests = v
		found = true
	}
	if v, ok := readInt("x-ratelimit-remaining-tokens"); ok {
		out.RemainingTokens = v
		found = true
	}
	if v, ok := readDur("x-ratelimit-reset-requests"); ok {
		out.ResetRequests = v
		found = true
	}
	if v, ok := readDur("x-ratelimit-reset-tokens"); ok {
		out.ResetTokens = v
		found = true
	}

	return out, found
}
