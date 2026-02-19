package restore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	insightifyv1 "insightify/gen/go/insightify/v1"
	uirepo "insightify/internal/gateway/repository/ui"
	gatewayuiworkspace "insightify/internal/gateway/service/uiworkspace"
)

const (
	ReasonResolved = insightifyv1.UiRestoreReason_UI_RESTORE_REASON_RESOLVED
	ReasonNoTab    = insightifyv1.UiRestoreReason_UI_RESTORE_REASON_NO_TAB
	ReasonNoRun    = insightifyv1.UiRestoreReason_UI_RESTORE_REASON_NO_RUN
	ReasonError    = insightifyv1.UiRestoreReason_UI_RESTORE_REASON_ERROR
)

type Result struct {
	ProjectID    string
	TabID        string
	RunID        string
	Reason       insightifyv1.UiRestoreReason
	Document     *insightifyv1.UiDocument
	DocumentHash string
}

func (r Result) IsResolved() bool {
	return r.Reason == ReasonResolved
}

type Service struct {
	uiStore    uirepo.Store
	workspaces *gatewayuiworkspace.Service
}

func New(uiStore uirepo.Store, workspaces *gatewayuiworkspace.Service) *Service {
	return &Service{
		uiStore:    uiStore,
		workspaces: workspaces,
	}
}

func (s *Service) ResolveProjectTabDocument(ctx context.Context, projectID, preferredTabID string) (Result, error) {
	if s == nil || s.uiStore == nil {
		return Result{}, fmt.Errorf("restore service ui store is not available")
	}
	if s.workspaces == nil {
		return Result{}, fmt.Errorf("restore service workspace is not available")
	}
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return Result{}, fmt.Errorf("project_id is required")
	}

	tabPref := strings.TrimSpace(preferredTabID)
	if tabPref == "" {
		tabPref = gatewayuiworkspace.DefaultTabID
	}

	_, tab, ok, err := s.workspaces.ResolveTab(pid, tabPref)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{
			ProjectID: pid,
			Reason:    ReasonNoTab,
		}, nil
	}

	tabID := strings.TrimSpace(tab.TabID)
	runID := strings.TrimSpace(tab.RunID)
	if runID == "" {
		return Result{
			ProjectID: pid,
			TabID:     tabID,
			Reason:    ReasonNoRun,
		}, nil
	}

	doc, err := s.uiStore.GetDocument(ctx, runID)
	if err != nil {
		return Result{}, err
	}
	return Result{
		ProjectID:    pid,
		TabID:        tabID,
		RunID:        runID,
		Reason:       ReasonResolved,
		Document:     doc,
		DocumentHash: HashDocumentCanonical(doc),
	}, nil
}

func (r Result) ToRestoreProtoResponse() *insightifyv1.RestoreUiResponse {
	return &insightifyv1.RestoreUiResponse{
		Reason:       r.Reason,
		ProjectId:    strings.TrimSpace(r.ProjectID),
		TabId:        strings.TrimSpace(r.TabID),
		RunId:        strings.TrimSpace(r.RunID),
		Document:     r.Document,
		DocumentHash: strings.TrimSpace(r.DocumentHash),
	}
}

func HashDocumentCanonical(doc *insightifyv1.UiDocument) string {
	if doc == nil {
		return ""
	}
	type encodedNode struct {
		id      string
		nodeTyp insightifyv1.UiNodeType
		payload []byte
	}
	nodes := make([]encodedNode, 0, len(doc.GetNodes()))
	for _, node := range doc.GetNodes() {
		if node == nil {
			continue
		}
		payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(node)
		if err != nil {
			continue
		}
		nodes = append(nodes, encodedNode{
			id:      strings.TrimSpace(node.GetId()),
			nodeTyp: node.GetType(),
			payload: payload,
		})
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].id != nodes[j].id {
			return nodes[i].id < nodes[j].id
		}
		if nodes[i].nodeTyp != nodes[j].nodeTyp {
			return nodes[i].nodeTyp < nodes[j].nodeTyp
		}
		return string(nodes[i].payload) < string(nodes[j].payload)
	})

	hasher := sha256.New()
	for _, node := range nodes {
		_, _ = hasher.Write([]byte(node.id))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(fmt.Sprintf("%d", node.nodeTyp)))
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write(node.payload)
		_, _ = hasher.Write([]byte{0})
	}
	sum := hasher.Sum(nil)
	return hex.EncodeToString(sum[:])
}
