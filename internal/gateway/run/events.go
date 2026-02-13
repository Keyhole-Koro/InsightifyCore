package run

import (
	"strings"
	"sync"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

const completedRunRetention = 30 * time.Second

// EventBroker manages per-run event channels.
type EventBroker struct {
	mu     sync.RWMutex
	events map[string]chan *insightifyv1.WatchRunResponse
}

// NewEventBroker creates a new event broker.
func NewEventBroker() *EventBroker {
	return &EventBroker{events: make(map[string]chan *insightifyv1.WatchRunResponse)}
}

// Allocate creates and registers a new event channel for a run.
func (b *EventBroker) Allocate(runID string, size int) chan *insightifyv1.WatchRunResponse {
	if size <= 0 {
		size = 1
	}
	ch := make(chan *insightifyv1.WatchRunResponse, size)
	b.mu.Lock()
	b.events[strings.TrimSpace(runID)] = ch
	b.mu.Unlock()
	return ch
}

// Get returns the event channel for a run.
func (b *EventBroker) Get(runID string) (chan *insightifyv1.WatchRunResponse, bool) {
	b.mu.RLock()
	ch, ok := b.events[strings.TrimSpace(runID)]
	b.mu.RUnlock()
	return ch, ok
}

// ScheduleCleanup removes a run's event channel after a retention period.
func (b *EventBroker) ScheduleCleanup(runID string) {
	time.AfterFunc(completedRunRetention, func() {
		b.mu.Lock()
		delete(b.events, strings.TrimSpace(runID))
		b.mu.Unlock()
	})
}
