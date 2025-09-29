package llm

import (
	"context"
	"time"
)

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
	// If rps is fractional, the period is sub-second (e.g., 1.5 rps ≈ 666ms).
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

// AcquireN acquires n tokens sequentially. If context is canceled during
// acquisition, it returns the error immediately.
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

// NewLimiter exposes a minimal Limiter backed by an internal rpsLimiter.
// If rps <= 0, the returned Limiter is nil and Acquire is a no-op when checked.
func NewLimiter(rps float64, burst int) Limiter {
	return newRPSLimiter(rps, burst)
}
