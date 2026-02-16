package uiworkspace

import (
	"context"
	"fmt"
	"time"

	"insightify/internal/gateway/ent"
	"insightify/internal/gateway/ent/workspace"
	"insightify/internal/gateway/ent/workspacetab"
)

type PostgresStore struct {
	client *ent.Client
}

func NewPostgresStore(client *ent.Client) *PostgresStore {
	return &PostgresStore{client: client}
}

func (s *PostgresStore) EnsureWorkspace(projectID string) (Workspace, error) {
	if s == nil || s.client == nil {
		return Workspace{}, fmt.Errorf("store is nil")
	}
	pid := normalizeProjectID(projectID)
	if pid == "" {
		return Workspace{}, fmt.Errorf("project_id is required")
	}

	ctx := context.Background()

	// Check if exists
	ws, err := s.client.Workspace.Query().
		Where(workspace.ProjectID(pid)).
		First(ctx)
	
	if err == nil {
		return entToWorkspace(ws), nil
	}
	if !ent.IsNotFound(err) {
		return Workspace{}, err
	}

	// Create
	wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
	err = s.client.Workspace.Create().
		SetID(wsID).
		SetProjectID(pid).
		SetName("Workspace").
		SetActiveTabID("").
		OnConflictColumns(workspace.FieldProjectID).
		Ignore(). // If race condition created it
		Exec(ctx)
	
	if err != nil {
		return Workspace{}, err
	}

	// Fetch again to be sure
	ws, err = s.client.Workspace.Query().
		Where(workspace.ProjectID(pid)).
		Only(ctx)
	if err != nil {
		return Workspace{}, err
	}
	return entToWorkspace(ws), nil
}

func (s *PostgresStore) GetWorkspaceByProject(projectID string) (Workspace, bool, error) {
	if s == nil || s.client == nil {
		return Workspace{}, false, fmt.Errorf("store is nil")
	}
	pid := normalizeProjectID(projectID)
	if pid == "" {
		return Workspace{}, false, fmt.Errorf("project_id is required")
	}

	ws, err := s.client.Workspace.Query().
		Where(workspace.ProjectID(pid)).
		Only(context.Background())
	
	if err != nil {
		if ent.IsNotFound(err) {
			return Workspace{}, false, nil
		}
		return Workspace{}, false, err
	}

	return entToWorkspace(ws), true, nil
}

func (s *PostgresStore) ListTabs(workspaceID string) ([]Tab, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("store is nil")
	}
	wid := normalizeWorkspaceID(workspaceID)
	if wid == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	tabs, err := s.client.WorkspaceTab.Query().
		Where(workspacetab.WorkspaceID(wid)).
		Order(ent.Asc(workspacetab.FieldOrderIndex), ent.Asc(workspacetab.FieldCreatedAt)).
		All(context.Background())
	
	if err != nil {
		return nil, err
	}

	out := make([]Tab, len(tabs))
	for i, t := range tabs {
		out[i] = entToTab(t)
	}
	return out, nil
}

func (s *PostgresStore) GetTab(workspaceID, tabID string) (Tab, bool, error) {
	if s == nil || s.client == nil {
		return Tab{}, false, fmt.Errorf("store is nil")
	}
	wid := normalizeWorkspaceID(workspaceID)
	tid := normalizeTabID(tabID)
	if wid == "" || tid == "" {
		return Tab{}, false, fmt.Errorf("workspace_id and tab_id are required")
	}

	t, err := s.client.WorkspaceTab.Query().
		Where(workspacetab.WorkspaceID(wid), workspacetab.ID(tid)).
		Only(context.Background())
	
	if err != nil {
		if ent.IsNotFound(err) {
			return Tab{}, false, nil
		}
		return Tab{}, false, err
	}

	return entToTab(t), true, nil
}

func (s *PostgresStore) CreateTab(workspaceID, title string) (Tab, error) {
	if s == nil || s.client == nil {
		return Tab{}, fmt.Errorf("store is nil")
	}
	wid := normalizeWorkspaceID(workspaceID)
	if wid == "" {
		return Tab{}, fmt.Errorf("workspace_id is required")
	}

	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Tab{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// Get max order
	maxOrder := 0
	// Ent doesn't support aggregate Max directly in fluent API easily without custom selector?
	// Actually it does via Aggregate. But for simplicity and assuming small number of tabs:
	lastTab, err := tx.WorkspaceTab.Query().
		Where(workspacetab.WorkspaceID(wid)).
		Order(ent.Desc(workspacetab.FieldOrderIndex)).
		First(ctx)
	
	if err == nil {
		maxOrder = lastTab.OrderIndex + 1
	}

	tid := fmt.Sprintf("tab-%d", time.Now().UnixNano())
	title = normalizeTitle(title)

	// Create Tab
	tab, err := tx.WorkspaceTab.Create().
		SetID(tid).
		SetWorkspaceID(wid).
		SetTitle(title).
		SetRunID("").
		SetOrderIndex(maxOrder).
		SetIsPinned(false).
		Save(ctx)
	if err != nil {
		return Tab{}, err
	}

	// Update Workspace active tab if empty?
	// The original SQL logic was: CASE WHEN active_tab_id = '' THEN $2 ELSE active_tab_id END
	// We need to read workspace to check active tab
	ws, err := tx.Workspace.Query().Where(workspace.ID(wid)).Only(ctx)
	if err != nil {
		return Tab{}, err
	}
	if ws.ActiveTabID == "" {
		_, err = tx.Workspace.UpdateOneID(wid).
			SetActiveTabID(tid).
			SetUpdatedAt(time.Now()).
			Save(ctx)
		if err != nil {
			return Tab{}, err
		}
	} else {
		_, err = tx.Workspace.UpdateOneID(wid).
			SetUpdatedAt(time.Now()).
			Save(ctx)
		if err != nil {
			return Tab{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Tab{}, err
	}

	return entToTab(tab), nil
}

func (s *PostgresStore) SelectTab(workspaceID, tabID string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("store is nil")
	}
	wid := normalizeWorkspaceID(workspaceID)
	tid := normalizeTabID(tabID)
	if wid == "" || tid == "" {
		return fmt.Errorf("workspace_id and tab_id are required")
	}

	ctx := context.Background()

	// Verify tab exists
	exists, err := s.client.WorkspaceTab.Query().
		Where(workspacetab.WorkspaceID(wid), workspacetab.ID(tid)).
		Exist(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("tab %s not found", tid)
	}

	// Update workspace
	_, err = s.client.Workspace.UpdateOneID(wid).
		SetActiveTabID(tid).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	return err
}

func (s *PostgresStore) UpdateTabRun(tabID, runID string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("store is nil")
	}
	tid := normalizeTabID(tabID)
	rid := normalizeRunID(runID)
	if tid == "" || rid == "" {
		return fmt.Errorf("tab_id and run_id are required")
	}

	// Update
	// Ent UpdateOneID returns error if not found? No, it returns error if not found on Save?
	// Actually UpdateOneID returns a builder. Save() returns the updated object or error.
	
	_, err := s.client.WorkspaceTab.UpdateOneID(tid).
		SetRunID(rid).
		SetUpdatedAt(time.Now()).
		Save(context.Background())
	
	if ent.IsNotFound(err) {
		return fmt.Errorf("tab %s not found", tid)
	}
	return err
}

// Helpers

// Helpers

func entToWorkspace(w *ent.Workspace) Workspace {
	return Workspace{
		WorkspaceID: w.ID,
		ProjectID:   w.ProjectID,
		Name:        w.Name,
		ActiveTabID: w.ActiveTabID,
	}
}

func entToTab(t *ent.WorkspaceTab) Tab {
	return Tab{
		TabID:           t.ID,
		WorkspaceID:     t.WorkspaceID,
		Title:           t.Title,
		RunID:           t.RunID,
		OrderIndex:      int32(t.OrderIndex),
		IsPinned:        t.IsPinned,
		CreatedAtUnixMs: t.CreatedAt.UnixMilli(),
	}
}
