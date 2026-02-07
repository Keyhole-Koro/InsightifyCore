package llmclient

import (
	"time"
)

// RateLimitHeaders represents normalized provider rate-limit signals.
type RateLimitHeaders struct {
	RetryAfterSeconds int

	LimitRequests     int
	LimitTokens       int
	RemainingRequests int
	RemainingTokens   int

	ResetRequests time.Duration
	ResetTokens   time.Duration
}

type RateLimitHeaderHandler func(headers RateLimitHeaders)

// RateLimitHeaderAwareClient is an optional interface for clients that expose
// parsed provider rate-limit headers.
type RateLimitHeaderAwareClient interface {
	SetRateLimitHeaderHandler(handler RateLimitHeaderHandler)
	LastRateLimitHeaders() (RateLimitHeaders, bool)
}

// RateLimitControlAdapter converts provider rate-limit signals to a wait duration.
// Middleware can use this adapter to throttle requests without knowing provider details.
type RateLimitControlAdapter interface {
	NextWait(headers RateLimitHeaders) time.Duration
}

// HeaderRateLimitControlAdapter provides generic control behavior for normalized signals.
type HeaderRateLimitControlAdapter struct{}

func (HeaderRateLimitControlAdapter) NextWait(headers RateLimitHeaders) time.Duration {
	if headers.RetryAfterSeconds > 0 {
		return time.Duration(headers.RetryAfterSeconds) * time.Second
	}
	if headers.RemainingTokens == 0 && headers.ResetTokens > 0 {
		return headers.ResetTokens
	}
	if headers.RemainingRequests == 0 && headers.ResetRequests > 0 {
		return headers.ResetRequests
	}
	return 0
}
