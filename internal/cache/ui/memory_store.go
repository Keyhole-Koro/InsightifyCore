package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

type docState struct {
	version int64
	nodes   map[string]*insightifyv1.UiNode
}

// MemoryStore manages the latest UI document for active runs in memory.
type MemoryStore struct {
	mu   sync.RWMutex
	docs map[string]*docState
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		docs: make(map[string]*docState),
	}
}

func (s *MemoryStore) GetDocument(_ context.Context, runID string) (*insightifyv1.UiDocument, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	key := normalizeRunID(runID)
	if key == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	s.mu.RLock()
	st := s.docs[key]
	s.mu.RUnlock()
	if st == nil {
		return &insightifyv1.UiDocument{RunId: key}, nil
	}
	return toDocument(key, st), nil
}

func (s *MemoryStore) ApplyOps(_ context.Context, runID string, baseVersion int64, ops []*insightifyv1.UiOp) (*insightifyv1.UiDocument, bool, error) {
	if s == nil {
		return nil, false, fmt.Errorf("store is nil")
	}
	key := normalizeRunID(runID)
	if key == "" {
		return nil, false, fmt.Errorf("run_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.docs[key]
	if st == nil {
		st = &docState{
			version: 0,
			nodes:   make(map[string]*insightifyv1.UiNode),
		}
		s.docs[key] = st
	}
	if baseVersion > 0 && baseVersion != st.version {
		return toDocument(key, st), true, nil
	}
	for _, op := range ops {
		if op == nil {
			continue
		}
		switch action := op.GetAction().(type) {
		case *insightifyv1.UiOp_UpsertNode:
			node := action.UpsertNode.GetNode()
			if node == nil {
				return nil, false, fmt.Errorf("upsert_node.node is required")
			}
			nodeID := strings.TrimSpace(node.GetId())
			if nodeID == "" {
				return nil, false, fmt.Errorf("upsert_node.node.id is required")
			}
			cloned, ok := proto.Clone(node).(*insightifyv1.UiNode)
			if !ok {
				return nil, false, fmt.Errorf("failed to clone ui node")
			}
			cloned.Id = nodeID
			st.nodes[nodeID] = cloned
		case *insightifyv1.UiOp_DeleteNode:
			nodeID := strings.TrimSpace(action.DeleteNode.GetNodeId())
			if nodeID == "" {
				return nil, false, fmt.Errorf("delete_node.node_id is required")
			}
			delete(st.nodes, nodeID)
		case *insightifyv1.UiOp_ClearNodes:
			st.nodes = make(map[string]*insightifyv1.UiNode)
		default:
			return nil, false, fmt.Errorf("unsupported ui op")
		}
	}
	if len(ops) > 0 {
		st.version++
	}
	return toDocument(key, st), false, nil
}

func normalizeRunID(runID string) string {
	return strings.TrimSpace(runID)
}

func toDocument(runID string, st *docState) *insightifyv1.UiDocument {
	if st == nil {
		return &insightifyv1.UiDocument{RunId: runID}
	}
	keys := make([]string, 0, len(st.nodes))
	for id := range st.nodes {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	nodes := make([]*insightifyv1.UiNode, 0, len(keys))
	for _, id := range keys {
		cloned, ok := proto.Clone(st.nodes[id]).(*insightifyv1.UiNode)
		if !ok {
			continue
		}
		nodes = append(nodes, cloned)
	}
	return &insightifyv1.UiDocument{
		RunId:   runID,
		Version: st.version,
		Nodes:   nodes,
	}
}
