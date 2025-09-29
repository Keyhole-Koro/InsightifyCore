package scheduler

import (
	"context"
	"sync/atomic"
	"testing"

	"insightify/internal/llm"
	"insightify/internal/tester"
)

type fakeBroker struct {
	reserved []int
	failAt   int32
	calls    int32
}

func (f *fakeBroker) Reserve(ctx context.Context, n int) (llm.Lease, error) {
	atomic.AddInt32(&f.calls, 1)
	f.reserved = append(f.reserved, n)
	return leaseCredits{n: n}, nil
}

type leaseCredits struct{ n int }

func (l leaseCredits) Context(ctx context.Context) context.Context { return llm.WithCredits(ctx, l.n) }

func TestScheduleHeavierStart_WithReservation(t *testing.T) {
	// Graph: 0->1->2, weights=1, cap=2 => chunks like [0,1], then [2] depending on desc priority.
	adj := [][]int{{1}, {2}, {}}
	weight := func(int) int { return 1 }
	targets := map[int]struct{}{0: {}, 1: {}, 2: {}}
	fb := &fakeBroker{}

	// Runner checks that it has credits equal to len(chunk)
	run := func(ctx context.Context, chunk []int) (<-chan struct{}, error) {
		// Consume exactly len(chunk) credits, then ensure no more.
		for i := 0; i < len(chunk); i++ {
			if ok := llm.TakeCredit(ctx); !ok {
				t.Fatalf("expected credit %d/%d in chunk", i+1, len(chunk))
			}
		}
		if llm.TakeCredit(ctx) {
			t.Fatalf("did not expect extra credits beyond chunk size")
		}
		ch := make(chan struct{})
		close(ch)
		return ch, nil
	}

	// Use WeightedPermits so each chunk reserves len(chunk) permits.
	err := ScheduleHeavierStart(context.Background(), Params{
		Adj:         adj,
		WeightOf:    WeightFn(weight),
		Targets:     targets,
		CapPerChunk: 2,
		NParallel:   1,
		Run:         ChunkRunner(run),
		Broker:      fb,
		ReserveWith: WeightedPermits(WeightFn(weight)),
	})
	tester.NoErr(t, err)
	tester.True(t, len(fb.reserved) > 0, "expected reservations to occur")
	for _, n := range fb.reserved {
		tester.True(t, n > 0, "reservation should be positive")
	}
}
