package llm

import (
    "context"
    "sync"
    "sync/atomic"
    "testing"

    "insightify/internal/tester"
)

func TestWithCreditsAndTakeCredit(t *testing.T) {
    ctx := context.Background()
    ctx = WithCredits(ctx, 10)

    var wg sync.WaitGroup
    var taken int64
    workers := 50
    wg.Add(workers)
    for i := 0; i < workers; i++ {
        go func() {
            defer wg.Done()
            for {
                if TakeCredit(ctx) {
                    atomic.AddInt64(&taken, 1)
                    continue
                }
                break
            }
        }()
    }
    wg.Wait()

    // After all attempts, no more credits should be available.
    tester.False(t, TakeCredit(ctx), "expected no credits left")
    tester.Eq(t, taken, int64(10), "exact number of credits consumed")
}

