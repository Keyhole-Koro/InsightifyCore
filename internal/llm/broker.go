package llm

import "context"

// Limiter is a minimal interface for an existing token/rps limiter.
type Limiter interface {
	Acquire(ctx context.Context) error
}

// PermitBroker reserves N permits up-front.
type PermitBroker interface {
	Reserve(ctx context.Context, n int) (Lease, error)
}

// Lease injects reserved credits into a context.
type Lease interface {
	Context(ctx context.Context) context.Context
}

type broker struct{ rl Limiter }

// NewBroker returns a PermitBroker backed by the given limiter.
func NewBroker(rl Limiter) PermitBroker { return &broker{rl: rl} }

// Reserve acquires n permits from the limiter and returns a lease that
// embeds n credits into a context. If any acquire fails, the error is returned.
// Unused credits are not returned; slight over-reservation is acceptable by design.
func (b *broker) Reserve(ctx context.Context, n int) (Lease, error) {
	if n <= 0 || b == nil || b.rl == nil {
		return lease{n: 0}, nil
	}
	for i := 0; i < n; i++ {
		if err := b.rl.Acquire(ctx); err != nil {
			return nil, err
		}
	}
	return lease{n: n}, nil
}

type lease struct{ n int }

// Context injects reserved credits into the provided context.
func (l lease) Context(ctx context.Context) context.Context { return WithCredits(ctx, l.n) }
