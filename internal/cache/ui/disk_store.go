package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

// DiskStore persists per-run UI documents on local disk.
type DiskStore struct {
	root string
	mu   sync.Mutex
}

func NewDiskStore(root string) *DiskStore {
	return &DiskStore{root: root}
}

func (s *DiskStore) GetDocument(ctx context.Context, runID string) (*insightifyv1.UiDocument, error) {
	_ = ctx
	runID = normalizeRunID(runID)
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(runID)
}

func (s *DiskStore) ApplyOps(ctx context.Context, runID string, baseVersion int64, ops []*insightifyv1.UiOp) (*insightifyv1.UiDocument, bool, error) {
	_ = ctx
	runID = normalizeRunID(runID)
	if runID == "" {
		return nil, false, fmt.Errorf("run_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.loadLocked(runID)
	if err != nil {
		return nil, false, err
	}
	st := &docState{
		version: doc.GetVersion(),
		nodes:   map[string]*insightifyv1.UiNode{},
	}
	for _, n := range doc.GetNodes() {
		st.nodes[n.GetId()] = n
	}
	if baseVersion > 0 && baseVersion != st.version {
		return toDocument(runID, st), true, nil
	}
	mem := &MemoryStore{docs: map[string]*docState{runID: st}}
	updated, conflict, err := mem.ApplyOps(context.Background(), runID, baseVersion, ops)
	if err != nil {
		return nil, false, err
	}
	if err := s.saveLocked(runID, updated); err != nil {
		return nil, false, err
	}
	return updated, conflict, nil
}

func (s *DiskStore) loadLocked(runID string) (*insightifyv1.UiDocument, error) {
	path, err := s.pathFor(runID)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &insightifyv1.UiDocument{RunId: runID}, nil
		}
		return nil, err
	}
	doc := &insightifyv1.UiDocument{}
	if err := protojson.Unmarshal(raw, doc); err != nil {
		return nil, err
	}
	if doc.RunId == "" {
		doc.RunId = runID
	}
	return doc, nil
}

func (s *DiskStore) saveLocked(runID string, doc *insightifyv1.UiDocument) error {
	path, err := s.pathFor(runID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	m := protojson.MarshalOptions{UseProtoNames: true}
	raw, err := m.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (s *DiskStore) pathFor(runID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("store is nil")
	}
	if s.root == "" {
		return "", fmt.Errorf("root is required")
	}
	return filepath.Join(s.root, runID+".json"), nil
}
