package uiworkspace

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
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
CREATE TABLE IF NOT EXISTS ui_workspaces (
    workspace_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL DEFAULT 'Workspace',
    active_tab_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS ui_workspace_tabs (
    tab_id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT 'Tab',
    run_id TEXT NOT NULL DEFAULT '',
    order_index INTEGER NOT NULL DEFAULT 0,
    is_pinned BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    FOREIGN KEY (workspace_id) REFERENCES ui_workspaces(workspace_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_ui_workspace_tabs_workspace_id ON ui_workspace_tabs(workspace_id);
`)
	})
	return s.schemaErr
}

func (s *PostgresStore) EnsureWorkspace(projectID string) (Workspace, error) {
	if s == nil {
		return Workspace{}, fmt.Errorf("store is nil")
	}
	if err := s.ensureSchema(); err != nil {
		return Workspace{}, err
	}
	pid := normalizeProjectID(projectID)
	if pid == "" {
		return Workspace{}, fmt.Errorf("project_id is required")
	}

	ws, ok, err := s.GetWorkspaceByProject(pid)
	if err != nil {
		return Workspace{}, err
	}
	if ok {
		return ws, nil
	}

	wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
	_, err = s.db.Exec(`
INSERT INTO ui_workspaces (workspace_id, project_id, name, active_tab_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, NOW(), NOW())
ON CONFLICT (project_id)
DO NOTHING
`, wsID, pid, "Workspace", "")
	if err != nil {
		return Workspace{}, err
	}
	ws, ok, err = s.GetWorkspaceByProject(pid)
	if err != nil {
		return Workspace{}, err
	}
	if !ok {
		return Workspace{}, fmt.Errorf("failed to ensure workspace for project %s", pid)
	}
	return ws, nil
}

func (s *PostgresStore) GetWorkspaceByProject(projectID string) (Workspace, bool, error) {
	if s == nil {
		return Workspace{}, false, fmt.Errorf("store is nil")
	}
	if err := s.ensureSchema(); err != nil {
		return Workspace{}, false, err
	}
	pid := normalizeProjectID(projectID)
	if pid == "" {
		return Workspace{}, false, fmt.Errorf("project_id is required")
	}
	row := s.db.QueryRow(`
SELECT workspace_id, project_id, name, active_tab_id
FROM ui_workspaces
WHERE project_id = $1
`, pid)
	var ws Workspace
	if err := row.Scan(&ws.WorkspaceID, &ws.ProjectID, &ws.Name, &ws.ActiveTabID); err != nil {
		if err == sql.ErrNoRows {
			return Workspace{}, false, nil
		}
		return Workspace{}, false, err
	}
	ws.WorkspaceID = normalizeWorkspaceID(ws.WorkspaceID)
	ws.ProjectID = normalizeProjectID(ws.ProjectID)
	ws.Name = strings.TrimSpace(ws.Name)
	ws.ActiveTabID = normalizeTabID(ws.ActiveTabID)
	return ws, true, nil
}

func (s *PostgresStore) ListTabs(workspaceID string) ([]Tab, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	if err := s.ensureSchema(); err != nil {
		return nil, err
	}
	wid := normalizeWorkspaceID(workspaceID)
	if wid == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	rows, err := s.db.Query(`
SELECT tab_id, workspace_id, title, run_id, order_index, is_pinned,
       COALESCE((EXTRACT(EPOCH FROM created_at) * 1000)::BIGINT, 0)
FROM ui_workspace_tabs
WHERE workspace_id = $1
ORDER BY order_index ASC, created_at ASC
`, wid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Tab, 0, 8)
	for rows.Next() {
		var t Tab
		if err := rows.Scan(&t.TabID, &t.WorkspaceID, &t.Title, &t.RunID, &t.OrderIndex, &t.IsPinned, &t.CreatedAtUnixMs); err != nil {
			continue
		}
		t.TabID = normalizeTabID(t.TabID)
		t.WorkspaceID = normalizeWorkspaceID(t.WorkspaceID)
		t.Title = normalizeTitle(t.Title)
		t.RunID = normalizeRunID(t.RunID)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *PostgresStore) GetTab(workspaceID, tabID string) (Tab, bool, error) {
	if s == nil {
		return Tab{}, false, fmt.Errorf("store is nil")
	}
	if err := s.ensureSchema(); err != nil {
		return Tab{}, false, err
	}
	wid := normalizeWorkspaceID(workspaceID)
	tid := normalizeTabID(tabID)
	if wid == "" || tid == "" {
		return Tab{}, false, fmt.Errorf("workspace_id and tab_id are required")
	}
	row := s.db.QueryRow(`
SELECT tab_id, workspace_id, title, run_id, order_index, is_pinned,
       COALESCE((EXTRACT(EPOCH FROM created_at) * 1000)::BIGINT, 0)
FROM ui_workspace_tabs
WHERE workspace_id = $1 AND tab_id = $2
`, wid, tid)
	var t Tab
	if err := row.Scan(&t.TabID, &t.WorkspaceID, &t.Title, &t.RunID, &t.OrderIndex, &t.IsPinned, &t.CreatedAtUnixMs); err != nil {
		if err == sql.ErrNoRows {
			return Tab{}, false, nil
		}
		return Tab{}, false, err
	}
	t.TabID = normalizeTabID(t.TabID)
	t.WorkspaceID = normalizeWorkspaceID(t.WorkspaceID)
	t.Title = normalizeTitle(t.Title)
	t.RunID = normalizeRunID(t.RunID)
	return t, true, nil
}

func (s *PostgresStore) CreateTab(workspaceID, title string) (Tab, error) {
	if s == nil {
		return Tab{}, fmt.Errorf("store is nil")
	}
	if err := s.ensureSchema(); err != nil {
		return Tab{}, err
	}
	wid := normalizeWorkspaceID(workspaceID)
	if wid == "" {
		return Tab{}, fmt.Errorf("workspace_id is required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return Tab{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var maxOrder sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(order_index) FROM ui_workspace_tabs WHERE workspace_id = $1`, wid).Scan(&maxOrder); err != nil {
		return Tab{}, err
	}
	nextOrder := int32(0)
	if maxOrder.Valid {
		nextOrder = int32(maxOrder.Int64 + 1)
	}

	tid := fmt.Sprintf("tab-%d", time.Now().UnixNano())
	title = normalizeTitle(title)
	if _, err := tx.Exec(`
INSERT INTO ui_workspace_tabs (tab_id, workspace_id, title, run_id, order_index, is_pinned, created_at, updated_at)
VALUES ($1, $2, $3, '', $4, FALSE, NOW(), NOW())
`, tid, wid, title, nextOrder); err != nil {
		return Tab{}, err
	}
	_, _ = tx.Exec(`
UPDATE ui_workspaces
SET active_tab_id = CASE WHEN active_tab_id = '' THEN $2 ELSE active_tab_id END,
    updated_at = NOW()
WHERE workspace_id = $1
`, wid, tid)

	if err := tx.Commit(); err != nil {
		return Tab{}, err
	}
	return Tab{
		TabID:           tid,
		WorkspaceID:     wid,
		Title:           title,
		RunID:           "",
		OrderIndex:      nextOrder,
		IsPinned:        false,
		CreatedAtUnixMs: time.Now().UnixMilli(),
	}, nil
}

func (s *PostgresStore) SelectTab(workspaceID, tabID string) error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	if err := s.ensureSchema(); err != nil {
		return err
	}
	wid := normalizeWorkspaceID(workspaceID)
	tid := normalizeTabID(tabID)
	if wid == "" || tid == "" {
		return fmt.Errorf("workspace_id and tab_id are required")
	}

	row := s.db.QueryRow(`
SELECT COUNT(1)
FROM ui_workspace_tabs
WHERE workspace_id = $1 AND tab_id = $2
`, wid, tid)
	var cnt int
	if err := row.Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		return fmt.Errorf("tab %s not found", tid)
	}

	_, err := s.db.Exec(`
UPDATE ui_workspaces
SET active_tab_id = $2, updated_at = NOW()
WHERE workspace_id = $1
`, wid, tid)
	return err
}

func (s *PostgresStore) UpdateTabRun(tabID, runID string) error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	if err := s.ensureSchema(); err != nil {
		return err
	}
	tid := normalizeTabID(tabID)
	rid := normalizeRunID(runID)
	if tid == "" || rid == "" {
		return fmt.Errorf("tab_id and run_id are required")
	}
	res, err := s.db.Exec(`
UPDATE ui_workspace_tabs
SET run_id = $2, updated_at = NOW()
WHERE tab_id = $1
`, tid, rid)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("tab %s not found", tid)
	}
	return nil
}
