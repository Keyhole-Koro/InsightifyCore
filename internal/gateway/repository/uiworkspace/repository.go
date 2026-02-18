package uiworkspace

import (
	"context"
	"strings"
)

// Workspace is a project-scoped UI container.
type Workspace struct {
	WorkspaceID string
	ProjectID   string
	Name        string
	ActiveTabID string
}

// Tab represents one UI tab in a workspace.
type Tab struct {
	TabID           string
	WorkspaceID     string
	Title           string
	RunID           string
	OrderIndex      int32
	IsPinned        bool
	CreatedAtUnixMs int64
}

// Store defines workspace/tab persistence operations.
type Store interface {
	EnsureWorkspace(ctx context.Context, projectID string) (Workspace, error)
	GetWorkspaceByProject(ctx context.Context, projectID string) (Workspace, bool, error)
	ListTabs(ctx context.Context, workspaceID string) ([]Tab, error)
	GetTab(ctx context.Context, workspaceID, tabID string) (Tab, bool, error)
	CreateTab(ctx context.Context, workspaceID, title string) (Tab, error)
	SelectTab(ctx context.Context, workspaceID, tabID string) error
	UpdateTabRun(ctx context.Context, tabID, runID string) error
}

func normalizeProjectID(v string) string   { return strings.TrimSpace(v) }
func normalizeWorkspaceID(v string) string { return strings.TrimSpace(v) }
func normalizeTabID(v string) string       { return strings.TrimSpace(v) }
func normalizeRunID(v string) string       { return strings.TrimSpace(v) }

func normalizeTitle(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "Tab"
	}
	return v
}
