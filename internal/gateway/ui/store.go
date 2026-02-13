package ui

import (
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

// Store manages the latest UI state for active runs in memory.
// It is thread-safe.
type Store struct {
	mu    sync.RWMutex
	nodes map[string]*insightifyv1.UiNode
}

// NewStore creates a new UI node store.
func NewStore() *Store {
	return &Store{
		nodes: make(map[string]*insightifyv1.UiNode),
	}
}

// Set stores or updates the UI node for a run.
func (s *Store) Set(runID string, node *insightifyv1.UiNode) {
	if s == nil || node == nil {
		return
	}
	s.mu.Lock()
	s.nodes[strings.TrimSpace(runID)] = node
	s.mu.Unlock()
}

// Get retrieves the latest UI node for a run.
// It returns a clone of the node to avoid data races.
func (s *Store) Get(runID string) *insightifyv1.UiNode {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	node := s.nodes[strings.TrimSpace(runID)]
	s.mu.RUnlock()
	if node == nil {
		return nil
	}
	cloned, ok := proto.Clone(node).(*insightifyv1.UiNode)
	if !ok {
		return nil
	}
	return cloned
}

// Clear removes the UI node for a run.
func (s *Store) Clear(runID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.nodes, strings.TrimSpace(runID))
	s.mu.Unlock()
}
