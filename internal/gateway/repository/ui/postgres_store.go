package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/ent"
	"insightify/internal/gateway/ent/userinteraction"
)

type PostgresStore struct {
	client *ent.Client
}

func NewPostgresStore(client *ent.Client) *PostgresStore {
	return &PostgresStore{client: client}
}

func (s *PostgresStore) GetDocument(runID string) *insightifyv1.UiDocument {
	if s == nil || s.client == nil {
		return nil
	}
	key := normalizeRunID(runID)
	if key == "" {
		return nil
	}

	doc, err := s.client.UserInteraction.Query().
		Where(userinteraction.ID(key)).
		Only(context.Background())
	
	if err != nil {
		if ent.IsNotFound(err) {
			return &insightifyv1.UiDocument{RunId: key}
		}
		// Log error?
		return &insightifyv1.UiDocument{RunId: key}
	}

	nodesMap, err := unmarshalNodes(doc.Nodes)
	if err != nil {
		return &insightifyv1.UiDocument{RunId: key}
	}

	return toDocFromMap(key, doc.Version, nodesMap)
}

func (s *PostgresStore) ApplyOps(runID string, baseVersion int64, ops []*insightifyv1.UiOp) (*insightifyv1.UiDocument, bool, error) {
	if s == nil || s.client == nil {
		return nil, false, fmt.Errorf("store is nil")
	}
	key := normalizeRunID(runID)
	if key == "" {
		return nil, false, fmt.Errorf("run_id is required")
	}

	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, false, err
	}
	
	// Lock/Get
	// Create if not exists (Upsert-like for init)
	err = tx.UserInteraction.Create().
		SetID(key).
		SetVersion(0).
		SetNodes(map[string]any{}).
		OnConflictColumns(userinteraction.FieldID).
		Ignore().
		Exec(ctx)
	if err != nil {
		tx.Rollback()
		return nil, false, err
	}

	// Fetch current state
	doc, err := tx.UserInteraction.Query().
		Where(userinteraction.ID(key)).
		// ForUpdate(). // Enable pessimistic locking if needed, removed for compatibility
		Only(ctx)
	if err != nil {
		tx.Rollback()
		return nil, false, err
	}

	// Consistency check
	if baseVersion > 0 && baseVersion != doc.Version {
		tx.Rollback()
		nodesMap, err := unmarshalNodes(doc.Nodes)
		if err != nil {
			return nil, false, fmt.Errorf("failed to parse existing state: %w", err)
		}
		return toDocFromMap(key, doc.Version, nodesMap), true, nil
	}

	// Load into memory
	nodesMap, err := unmarshalNodes(doc.Nodes)
	if err != nil {
		tx.Rollback()
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
				tx.Rollback()
				return nil, false, fmt.Errorf("upsert_node.node is required")
			}
			id := strings.TrimSpace(node.GetId())
			if id == "" {
				tx.Rollback()
				return nil, false, fmt.Errorf("upsert_node.node.id is required")
			}
			cloned, ok := proto.Clone(node).(*insightifyv1.UiNode)
			if !ok {
				tx.Rollback()
				return nil, false, fmt.Errorf("failed to clone ui node")
			}
			cloned.Id = id
			nodesMap[id] = cloned
			changed = true
		case *insightifyv1.UiOp_DeleteNode:
			id := strings.TrimSpace(action.DeleteNode.GetNodeId())
			if id == "" {
				tx.Rollback()
				return nil, false, fmt.Errorf("delete_node.node_id is required")
			}
			delete(nodesMap, id)
			changed = true
		case *insightifyv1.UiOp_ClearNodes:
			nodesMap = make(map[string]*insightifyv1.UiNode)
			changed = true
		default:
			tx.Rollback()
			return nil, false, fmt.Errorf("unsupported ui op")
		}
	}

	if changed {
		newVersion := doc.Version + 1
		newJSON, err := marshalNodes(nodesMap) // This returns []byte, Ent needs map[string]any for JSON field
		if err != nil {
			tx.Rollback()
			return nil, false, fmt.Errorf("failed to marshal new state: %w", err)
		}
		
		// Convert []byte back to map[string]any for Ent
		var jsonMap map[string]any
		if err := json.Unmarshal(newJSON, &jsonMap); err != nil {
			tx.Rollback()
			return nil, false, err
		}

		_, err = tx.UserInteraction.UpdateOneID(key).
			SetVersion(newVersion).
			SetNodes(jsonMap).
			SetUpdatedAt(time.Now()).
			Save(ctx)
		if err != nil {
			tx.Rollback()
			return nil, false, err
		}
		doc.Version = newVersion
	}

	if err := tx.Commit(); err != nil {
		return nil, false, err
	}

	return toDocFromMap(key, doc.Version, nodesMap), false, nil
}

// Helpers

func unmarshalNodes(data any) (map[string]*insightifyv1.UiNode, error) {
	// Ent stores as map[string]any in memory check
	// If it comes from DB, it might be unmarshaled to map[string]any
	
	// We need to handle map[string]any -> map[string]*UiNode conversion via JSON
	
	bytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(bytes, &rawMap); err != nil {
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

