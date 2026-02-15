package ui

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

type PostgresStore struct {
	db         *sql.DB
	schemaOnce sync.Once
	schemaErr  error
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) ensureSchema() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("db is nil")
	}
	s.schemaOnce.Do(func() {
		_, s.schemaErr = s.db.Exec(`
CREATE TABLE IF NOT EXISTS ui_documents (
    run_id TEXT PRIMARY KEY,
    version BIGINT NOT NULL DEFAULT 0,
    nodes JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
`)
	})
	return s.schemaErr
}

func (s *PostgresStore) GetDocument(runID string) *insightifyv1.UiDocument {
	if s == nil {
		return nil
	}
	key := normalizeRunID(runID)
	if key == "" {
		return nil
	}
	if err := s.ensureSchema(); err != nil {
		// Log error? For now just return empty (interface contract doesn't return error for Get)
		return &insightifyv1.UiDocument{RunId: key}
	}
	row := s.db.QueryRow(`SELECT version, nodes FROM ui_documents WHERE run_id = $1`, key)
	var version int64
	var nodesJSON []byte
	if err := row.Scan(&version, &nodesJSON); err != nil {
		if err == sql.ErrNoRows {
			return &insightifyv1.UiDocument{RunId: key}
		}
		return &insightifyv1.UiDocument{RunId: key}
	}

	nodesMap, err := unmarshalNodes(nodesJSON)
	if err != nil {
		return &insightifyv1.UiDocument{RunId: key}
	}

	return toDocFromMap(key, version, nodesMap)
}

func (s *PostgresStore) ApplyOps(runID string, baseVersion int64, ops []*insightifyv1.UiOp) (*insightifyv1.UiDocument, bool, error) {
	if s == nil {
		return nil, false, fmt.Errorf("store is nil")
	}
	key := normalizeRunID(runID)
	if key == "" {
		return nil, false, fmt.Errorf("run_id is required")
	}
	if err := s.ensureSchema(); err != nil {
		return nil, false, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	// Ensure and then lock one row per run_id to avoid races on first insert.
	if _, err := tx.Exec(`INSERT INTO ui_documents (run_id) VALUES ($1) ON CONFLICT (run_id) DO NOTHING`, key); err != nil {
		return nil, false, err
	}
	row := tx.QueryRow(`SELECT version, nodes FROM ui_documents WHERE run_id = $1 FOR UPDATE`, key)
	var version int64
	var nodesJSON []byte
	if err := row.Scan(&version, &nodesJSON); err != nil {
		return nil, false, err
	}

	// Consistency check
	if baseVersion > 0 && baseVersion != version {
		nodesMap, err := unmarshalNodes(nodesJSON)
		if err != nil {
			return nil, false, fmt.Errorf("failed to parse existing state: %w", err)
		}
		doc := toDocFromMap(key, version, nodesMap)
		return doc, true, nil
	}

	// Load into memory
	nodesMap, err := unmarshalNodes(nodesJSON)
	if err != nil {
		return nil, false, fmt.Errorf("failed to parse existing state: %w", err)
	}

	// Apply ops
	changed := false
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
			id := strings.TrimSpace(node.GetId())
			if id == "" {
				return nil, false, fmt.Errorf("upsert_node.node.id is required")
			}
			cloned, ok := proto.Clone(node).(*insightifyv1.UiNode)
			if !ok {
				return nil, false, fmt.Errorf("failed to clone ui node")
			}
			cloned.Id = id
			nodesMap[id] = cloned
			changed = true
		case *insightifyv1.UiOp_DeleteNode:
			id := strings.TrimSpace(action.DeleteNode.GetNodeId())
			if id == "" {
				return nil, false, fmt.Errorf("delete_node.node_id is required")
			}
			delete(nodesMap, id)
			changed = true
		case *insightifyv1.UiOp_ClearNodes:
			nodesMap = make(map[string]*insightifyv1.UiNode)
			changed = true
		default:
			return nil, false, fmt.Errorf("unsupported ui op")
		}
	}

	if changed {
		version++
		newJSON, err := marshalNodes(nodesMap)
		if err != nil {
			return nil, false, fmt.Errorf("failed to marshal new state: %w", err)
		}
		_, err = tx.Exec(`UPDATE ui_documents SET version=$2, nodes=$3, updated_at=$4 WHERE run_id=$1`,
			key, version, newJSON, time.Now())
		if err != nil {
			return nil, false, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, false, err
	}

	return toDocFromMap(key, version, nodesMap), false, nil
}

// Helpers

func unmarshalNodes(data []byte) (map[string]*insightifyv1.UiNode, error) {
	if len(data) == 0 {
		return make(map[string]*insightifyv1.UiNode), nil
	}
	// We store as a JSON object: {"node-id": {UiNode content}, ...}
	// protojson doesn't strictly support map[string]Message unmarshaling directly via Unmarshal?
	// Actually, standard json can unmarshal into map[string]json.RawMessage, then we loops.
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return nil, err
	}
	out := make(map[string]*insightifyv1.UiNode, len(rawMap))
	for k, v := range rawMap {
		node := &insightifyv1.UiNode{}
		if err := protojson.Unmarshal(v, node); err != nil {
			return nil, err
		}
		out[k] = node
	}
	return out, nil
}

func marshalNodes(nodes map[string]*insightifyv1.UiNode) ([]byte, error) {
	// Convert to map[string]json.RawMessage or just manual JSON build
	// Better to use a struct that we can marshal
	// Using map[string]any might invoke default json marshaling which won't use protojson for the values.
	// We need to marshal each node individually.
	// Efficient approach: build a map[string]json.RawMessage
	rawMap := make(map[string]json.RawMessage, len(nodes))
	opts := protojson.MarshalOptions{UseProtoNames: true}
	for k, v := range nodes {
		b, err := opts.Marshal(v)
		if err != nil {
			return nil, err
		}
		rawMap[k] = b
	}
	return json.Marshal(rawMap)
}

func toDocFromMap(runID string, version int64, nodes map[string]*insightifyv1.UiNode) *insightifyv1.UiDocument {
	keys := make([]string, 0, len(nodes))
	for k := range nodes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	outNodes := make([]*insightifyv1.UiNode, 0, len(keys))
	for _, k := range keys {
		cloned, ok := proto.Clone(nodes[k]).(*insightifyv1.UiNode)
		if !ok {
			continue
		}
		outNodes = append(outNodes, cloned)
	}
	return &insightifyv1.UiDocument{
		RunId:   runID,
		Version: version,
		Nodes:   outNodes,
	}
}
