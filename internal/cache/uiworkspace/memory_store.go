package uiworkspace

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-memory origin/fallback for workspace store contract.
type MemoryStore struct {
	mu              sync.Mutex
	workspaces      map[string]Workspace
	workspaceByProj map[string]string
	tabsByWorkspace map[string][]Tab
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		workspaces:      make(map[string]Workspace),
		workspaceByProj: make(map[string]string),
		tabsByWorkspace: make(map[string][]Tab),
	}
}

func (s *MemoryStore) EnsureWorkspace(_ context.Context, projectID string) (Workspace, error) {
	if s == nil {
		return Workspace{}, fmt.Errorf("store is nil")
	}
	pid := normalizeProjectID(projectID)
	if pid == "" {
		return Workspace{}, fmt.Errorf("project_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if wsID, ok := s.workspaceByProj[pid]; ok {
		return s.workspaces[wsID], nil
	}
	ws := Workspace{
		WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
		ProjectID:   pid,
		Name:        "Workspace",
	}
	s.workspaces[ws.WorkspaceID] = ws
	s.workspaceByProj[pid] = ws.WorkspaceID
	return ws, nil
}

func (s *MemoryStore) GetWorkspaceByProject(_ context.Context, projectID string) (Workspace, bool, error) {
	if s == nil {
		return Workspace{}, false, fmt.Errorf("store is nil")
	}
	pid := normalizeProjectID(projectID)
	if pid == "" {
		return Workspace{}, false, fmt.Errorf("project_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	wsID, ok := s.workspaceByProj[pid]
	if !ok {
		return Workspace{}, false, nil
	}
	ws, ok := s.workspaces[wsID]
	return ws, ok, nil
}

func (s *MemoryStore) ListTabs(_ context.Context, workspaceID string) ([]Tab, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	wid := normalizeWorkspaceID(workspaceID)
	if wid == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tabs := append([]Tab(nil), s.tabsByWorkspace[wid]...)
	sort.Slice(tabs, func(i, j int) bool {
		if tabs[i].OrderIndex == tabs[j].OrderIndex {
			return tabs[i].CreatedAtUnixMs < tabs[j].CreatedAtUnixMs
		}
		return tabs[i].OrderIndex < tabs[j].OrderIndex
	})
	return tabs, nil
}

func (s *MemoryStore) GetTab(_ context.Context, workspaceID, tabID string) (Tab, bool, error) {
	if s == nil {
		return Tab{}, false, fmt.Errorf("store is nil")
	}
	wid := normalizeWorkspaceID(workspaceID)
	tid := normalizeTabID(tabID)
	if wid == "" || tid == "" {
		return Tab{}, false, fmt.Errorf("workspace_id and tab_id are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tabsByWorkspace[wid] {
		if t.TabID == tid {
			return t, true, nil
		}
	}
	return Tab{}, false, nil
}

func (s *MemoryStore) CreateTab(_ context.Context, workspaceID, title string) (Tab, error) {
	if s == nil {
		return Tab{}, fmt.Errorf("store is nil")
	}
	wid := normalizeWorkspaceID(workspaceID)
	if wid == "" {
		return Tab{}, fmt.Errorf("workspace_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ws, ok := s.workspaces[wid]
	if !ok {
		return Tab{}, fmt.Errorf("workspace %s not found", wid)
	}
	order := int32(len(s.tabsByWorkspace[wid]))
	tab := Tab{
		TabID:           fmt.Sprintf("tab-%d", time.Now().UnixNano()),
		WorkspaceID:     wid,
		Title:           normalizeTitle(title),
		OrderIndex:      order,
		CreatedAtUnixMs: time.Now().UnixMilli(),
	}
	s.tabsByWorkspace[wid] = append(s.tabsByWorkspace[wid], tab)
	if ws.ActiveTabID == "" {
		ws.ActiveTabID = tab.TabID
		s.workspaces[wid] = ws
	}
	return tab, nil
}

func (s *MemoryStore) SelectTab(_ context.Context, workspaceID, tabID string) error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	wid := normalizeWorkspaceID(workspaceID)
	tid := normalizeTabID(tabID)
	if wid == "" || tid == "" {
		return fmt.Errorf("workspace_id and tab_id are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ws, ok := s.workspaces[wid]
	if !ok {
		return fmt.Errorf("workspace %s not found", wid)
	}
	found := false
	for _, t := range s.tabsByWorkspace[wid] {
		if t.TabID == tid {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("tab %s not found", tid)
	}
	ws.ActiveTabID = tid
	s.workspaces[wid] = ws
	return nil
}

func (s *MemoryStore) UpdateTabRun(_ context.Context, tabID, runID string) error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	tid := normalizeTabID(tabID)
	rid := normalizeRunID(runID)
	if tid == "" || rid == "" {
		return fmt.Errorf("tab_id and run_id are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for wid, tabs := range s.tabsByWorkspace {
		for i := range tabs {
			if tabs[i].TabID == tid {
				tabs[i].RunID = rid
				s.tabsByWorkspace[wid] = tabs
				return nil
			}
		}
	}
	return fmt.Errorf("tab %s not found", tid)
}
