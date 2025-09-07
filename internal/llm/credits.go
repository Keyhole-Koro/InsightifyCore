package llm

import (
	"context"
	"sync/atomic"
)

// creditsKey is an unexported context key type for carrying reserved credits.
type creditsKey struct{}

// credits holds an atomic counter of available credits.
type credits struct{ n int32 }

// WithCredits returns a context that carries n consumable credits.
// If n <= 0, the original context is returned.
func WithCredits(ctx context.Context, n int) context.Context {
	if n <= 0 {
		return ctx
	}
	c := &credits{n: int32(n)}
	return context.WithValue(ctx, creditsKey{}, c)
}

// TakeCredit atomically consumes one credit from the context if available.
// Returns true when a credit was consumed; false otherwise.
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
