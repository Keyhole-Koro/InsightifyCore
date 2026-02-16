package artifact

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type MemoryStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string][]byte),
	}
}

func (s *MemoryStore) Put(_ context.Context, runID, path string, content []byte) error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	runID = strings.TrimSpace(runID)
	path = strings.TrimSpace(path)
	if runID == "" {
		return fmt.Errorf("run_id is required")
	}
	if path == "" {
		return fmt.Errorf("path is required")
	}
	key := runID + "/" + strings.TrimLeft(path, "/")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = append([]byte(nil), content...)
	return nil
}

func (s *MemoryStore) Get(_ context.Context, runID, path string) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	runID = strings.TrimSpace(runID)
	path = strings.TrimSpace(path)
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	key := runID + "/" + strings.TrimLeft(path, "/")
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok := s.data[key]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), raw...), nil
}

func (s *MemoryStore) List(_ context.Context, runID string) ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	prefix := runID + "/"
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, 16)
	for key := range s.data {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		out = append(out, strings.TrimPrefix(key, prefix))
	}
	sort.Strings(out)
	return out, nil
}

func (s *MemoryStore) GetURL(ctx context.Context, runID, path string) (string, error) {
	// Memory store doesn't support URLs, return path as fallback or empty
	return "", nil
}
