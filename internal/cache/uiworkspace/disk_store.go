package uiworkspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type diskSnapshot struct {
	Workspaces      map[string]Workspace `json:"workspaces"`
	WorkspaceByProj map[string]string    `json:"workspace_by_project"`
	TabsByWorkspace map[string][]Tab     `json:"tabs_by_workspace"`
}

// DiskStore persists workspace/tab state into a local JSON file.
type DiskStore struct {
	path string

	loadOnce sync.Once
	mu       sync.Mutex

	workspaces      map[string]Workspace
	workspaceByProj map[string]string
	tabsByWorkspace map[string][]Tab
}

func NewDiskStore(path string) *DiskStore {
	return &DiskStore{
		path:            path,
		workspaces:      map[string]Workspace{},
		workspaceByProj: map[string]string{},
		tabsByWorkspace: map[string][]Tab{},
	}
}

func (s *DiskStore) EnsureWorkspace(_ context.Context, projectID string) (Workspace, error) {
	if s == nil {
		return Workspace{}, fmt.Errorf("store is nil")
	}
	pid := normalizeProjectID(projectID)
	if pid == "" {
		return Workspace{}, fmt.Errorf("project_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
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
	return ws, s.saveLocked()
}

func (s *DiskStore) GetWorkspaceByProject(_ context.Context, projectID string) (Workspace, bool, error) {
	if s == nil {
		return Workspace{}, false, fmt.Errorf("store is nil")
	}
	pid := normalizeProjectID(projectID)
	if pid == "" {
		return Workspace{}, false, fmt.Errorf("project_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	wsID, ok := s.workspaceByProj[pid]
	if !ok {
		return Workspace{}, false, nil
	}
	ws, ok := s.workspaces[wsID]
	return ws, ok, nil
}

func (s *DiskStore) ListTabs(_ context.Context, workspaceID string) ([]Tab, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	wid := normalizeWorkspaceID(workspaceID)
	if wid == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	tabs := append([]Tab(nil), s.tabsByWorkspace[wid]...)
	sort.Slice(tabs, func(i, j int) bool {
		if tabs[i].OrderIndex == tabs[j].OrderIndex {
			return tabs[i].CreatedAtUnixMs < tabs[j].CreatedAtUnixMs
		}
		return tabs[i].OrderIndex < tabs[j].OrderIndex
	})
	return tabs, nil
}

func (s *DiskStore) GetTab(_ context.Context, workspaceID, tabID string) (Tab, bool, error) {
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
	s.ensureLoadedLocked()
	for _, t := range s.tabsByWorkspace[wid] {
		if t.TabID == tid {
			return t, true, nil
		}
	}
	return Tab{}, false, nil
}

func (s *DiskStore) CreateTab(_ context.Context, workspaceID, title string) (Tab, error) {
	if s == nil {
		return Tab{}, fmt.Errorf("store is nil")
	}
	wid := normalizeWorkspaceID(workspaceID)
	if wid == "" {
		return Tab{}, fmt.Errorf("workspace_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
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
	return tab, s.saveLocked()
}

func (s *DiskStore) SelectTab(_ context.Context, workspaceID, tabID string) error {
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
	s.ensureLoadedLocked()
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
	return s.saveLocked()
}

func (s *DiskStore) UpdateTabRun(_ context.Context, tabID, runID string) error {
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
	s.ensureLoadedLocked()
	for wid, tabs := range s.tabsByWorkspace {
		for i := range tabs {
			if tabs[i].TabID == tid {
				tabs[i].RunID = rid
				s.tabsByWorkspace[wid] = tabs
				return s.saveLocked()
			}
		}
	}
	return fmt.Errorf("tab %s not found", tid)
}

func (s *DiskStore) ensureLoadedLocked() {
	s.loadOnce.Do(func() {
		raw, err := os.ReadFile(s.path)
		if err != nil {
			return
		}
		var snap diskSnapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			return
		}
		if snap.Workspaces != nil {
			s.workspaces = snap.Workspaces
		}
		if snap.WorkspaceByProj != nil {
			s.workspaceByProj = snap.WorkspaceByProj
		}
		if snap.TabsByWorkspace != nil {
			s.tabsByWorkspace = snap.TabsByWorkspace
		}
	})
}

func (s *DiskStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(diskSnapshot{
		Workspaces:      s.workspaces,
		WorkspaceByProj: s.workspaceByProj,
		TabsByWorkspace: s.tabsByWorkspace,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0o644)
}
