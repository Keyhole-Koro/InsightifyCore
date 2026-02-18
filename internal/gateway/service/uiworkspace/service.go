package uiworkspace

import (
	"context"
	"fmt"
	"strings"

	repo "insightify/internal/gateway/repository/uiworkspace"
)

const DefaultTabID = "default"

const defaultTabTitle = "Default"

type Service struct {
	store repo.Store
}

func New(store repo.Store) *Service {
	return &Service{store: store}
}

type WorkspaceView struct {
	Workspace repo.Workspace
	Tabs      []repo.Tab
}

func (s *Service) Ensure(projectID string) (WorkspaceView, error) {
	if s == nil || s.store == nil {
		return WorkspaceView{}, fmt.Errorf("ui workspace service is not available")
	}
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return WorkspaceView{}, fmt.Errorf("project_id is required")
	}
	ws, err := s.store.EnsureWorkspace(context.Background(), pid)
	if err != nil {
		return WorkspaceView{}, err
	}
	tabs, err := s.store.ListTabs(context.Background(), ws.WorkspaceID)
	if err != nil {
		return WorkspaceView{}, err
	}
	if len(tabs) == 0 {
		tab, err := s.store.CreateTab(context.Background(), ws.WorkspaceID, defaultTabTitle)
		if err != nil {
			return WorkspaceView{}, err
		}
		tabs = []repo.Tab{tab}
		if ws.ActiveTabID == "" {
			ws.ActiveTabID = tab.TabID
		}
	}
	if strings.TrimSpace(ws.ActiveTabID) == "" {
		ws.ActiveTabID = tabs[len(tabs)-1].TabID
		_ = s.store.SelectTab(context.Background(), ws.WorkspaceID, ws.ActiveTabID)
	}
	return WorkspaceView{Workspace: ws, Tabs: tabs}, nil
}

func (s *Service) ResolveTab(projectID, preferredTabID string) (repo.Workspace, repo.Tab, bool, error) {
	view, err := s.Ensure(projectID)
	if err != nil {
		return repo.Workspace{}, repo.Tab{}, false, err
	}
	if len(view.Tabs) == 0 {
		return repo.Workspace{}, repo.Tab{}, false, nil
	}
	target := strings.TrimSpace(preferredTabID)
	if target == "" || strings.EqualFold(target, DefaultTabID) {
		if defaultTab, ok := findDefaultTab(view.Tabs); ok {
			return view.Workspace, defaultTab, true, nil
		}
		created, err := s.store.CreateTab(context.Background(), view.Workspace.WorkspaceID, defaultTabTitle)
		if err != nil {
			return repo.Workspace{}, repo.Tab{}, false, err
		}
		_ = s.store.SelectTab(context.Background(), view.Workspace.WorkspaceID, created.TabID)
		view.Workspace.ActiveTabID = created.TabID
		return view.Workspace, created, true, nil
	}
	if target != "" {
		for _, t := range view.Tabs {
			if strings.TrimSpace(t.TabID) == target {
				return view.Workspace, t, true, nil
			}
		}
	}
	active := strings.TrimSpace(view.Workspace.ActiveTabID)
	if active != "" {
		for _, t := range view.Tabs {
			if strings.TrimSpace(t.TabID) == active {
				return view.Workspace, t, true, nil
			}
		}
	}
	return view.Workspace, view.Tabs[len(view.Tabs)-1], true, nil
}

func findDefaultTab(tabs []repo.Tab) (repo.Tab, bool) {
	for _, t := range tabs {
		if strings.EqualFold(strings.TrimSpace(t.Title), defaultTabTitle) {
			return t, true
		}
	}
	return repo.Tab{}, false
}

func (s *Service) AttachRunToCurrentTab(projectID, runID string) (repo.Tab, error) {
	_, tab, ok, err := s.ResolveTab(projectID, "")
	if err != nil {
		return repo.Tab{}, err
	}
	if !ok {
		return repo.Tab{}, fmt.Errorf("no tab available")
	}
	if err := s.store.UpdateTabRun(context.Background(), tab.TabID, runID); err != nil {
		return repo.Tab{}, err
	}
	tab.RunID = strings.TrimSpace(runID)
	return tab, nil
}

func (s *Service) AssignRunToCurrentTab(projectID, runID string) error {
	_, err := s.AttachRunToCurrentTab(projectID, runID)
	return err
}

func (s *Service) CreateTab(projectID, title string) (WorkspaceView, repo.Tab, error) {
	view, err := s.Ensure(projectID)
	if err != nil {
		return WorkspaceView{}, repo.Tab{}, err
	}
	tab, err := s.store.CreateTab(context.Background(), view.Workspace.WorkspaceID, title)
	if err != nil {
		return WorkspaceView{}, repo.Tab{}, err
	}
	_ = s.store.SelectTab(context.Background(), view.Workspace.WorkspaceID, tab.TabID)
	updated, err := s.Ensure(projectID)
	if err != nil {
		return WorkspaceView{}, repo.Tab{}, err
	}
	return updated, tab, nil
}

func (s *Service) SelectTab(projectID, tabID string) (WorkspaceView, error) {
	view, err := s.Ensure(projectID)
	if err != nil {
		return WorkspaceView{}, err
	}
	if err := s.store.SelectTab(context.Background(), view.Workspace.WorkspaceID, tabID); err != nil {
		return WorkspaceView{}, err
	}
	return s.Ensure(projectID)
}
