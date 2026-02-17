package llm

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"insightify/internal/tester"
)

type fakeLimiter struct {
	calls  int32
	failAt int32
}

func (f *fakeLimiter) Acquire(ctx context.Context) error {
	n := atomic.AddInt32(&f.calls, 1)
	if f.failAt > 0 && n == f.failAt {
		return errors.New("boom")
	}
	return nil
}

func TestBrokerReserveSuccess(t *testing.T) {
	fl := &fakeLimiter{}
	b := NewBroker(fl)
	lease, err := b.Reserve(context.Background(), 3)
	tester.NoErr(t, err)
	tester.Eq(t, fl.calls, int32(3))
	ctx := lease.Context(context.Background())
	// Consume exactly 3 credits
	for i := 0; i < 3; i++ {
		tester.True(t, TakeCredit(ctx), "credit available")
	}
	tester.False(t, TakeCredit(ctx), "no extra credits")
}

func TestBrokerReserveError(t *testing.T) {
	fl := &fakeLimiter{failAt: 2}
	b := NewBroker(fl)
	_, err := b.Reserve(context.Background(), 3)
	if err == nil {
		t.Fatalf("expected error on reservation")
	}
	tester.Eq(t, fl.calls, int32(2))
}
