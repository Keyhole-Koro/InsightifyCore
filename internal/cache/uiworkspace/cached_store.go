package uiworkspace

import (
	"context"
	"strings"
	"time"

	memcache "insightify/internal/cache/memory"
	uiworkspacerepo "insightify/internal/gateway/repository/uiworkspace"
)

type Store = uiworkspacerepo.Store
type Workspace = uiworkspacerepo.Workspace
type Tab = uiworkspacerepo.Tab

type CacheConfig struct {
	TTL        time.Duration
	MaxEntries int
}

func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		TTL:        2 * time.Minute,
		MaxEntries: 2048,
	}
}

type CachedStore struct {
	origin Store

	workspaceByProject *memcache.LRUTTL[string, Workspace]
	workspaceByID      *memcache.LRUTTL[string, Workspace]
	tabsByWorkspace    *memcache.LRUTTL[string, []Tab]
	tabByKey           *memcache.LRUTTL[string, Tab]
	tabToWorkspace     *memcache.LRUTTL[string, string]
}

func NewCachedStore(origin Store, cfg CacheConfig) *CachedStore {
	if cfg.TTL <= 0 || cfg.MaxEntries <= 0 {
		def := DefaultCacheConfig()
		if cfg.TTL <= 0 {
			cfg.TTL = def.TTL
		}
		if cfg.MaxEntries <= 0 {
			cfg.MaxEntries = def.MaxEntries
		}
	}
	return &CachedStore{
		origin:             origin,
		workspaceByProject: memcache.NewLRUTTL[string, Workspace](cfg.MaxEntries, 0, cfg.TTL),
		workspaceByID:      memcache.NewLRUTTL[string, Workspace](cfg.MaxEntries, 0, cfg.TTL),
		tabsByWorkspace:    memcache.NewLRUTTL[string, []Tab](cfg.MaxEntries, 0, cfg.TTL),
		tabByKey:           memcache.NewLRUTTL[string, Tab](cfg.MaxEntries, 0, cfg.TTL),
		tabToWorkspace:     memcache.NewLRUTTL[string, string](cfg.MaxEntries, 0, cfg.TTL),
	}
}

func (s *CachedStore) EnsureWorkspace(ctx context.Context, projectID string) (Workspace, error) {
	ws, err := s.origin.EnsureWorkspace(ctx, projectID)
	if err != nil {
		return Workspace{}, err
	}
	s.cacheWorkspace(ws)
	return ws, nil
}

func (s *CachedStore) GetWorkspaceByProject(ctx context.Context, projectID string) (Workspace, bool, error) {
	k := strings.TrimSpace(projectID)
	if ws, ok := s.workspaceByProject.Get(k); ok {
		return ws, true, nil
	}
	ws, ok, err := s.origin.GetWorkspaceByProject(ctx, k)
	if err != nil || !ok {
		return ws, ok, err
	}
	s.cacheWorkspace(ws)
	return ws, true, nil
}

func (s *CachedStore) ListTabs(ctx context.Context, workspaceID string) ([]Tab, error) {
	wid := strings.TrimSpace(workspaceID)
	if tabs, ok := s.tabsByWorkspace.Get(wid); ok {
		return cloneTabs(tabs), nil
	}
	tabs, err := s.origin.ListTabs(ctx, wid)
	if err != nil {
		return nil, err
	}
	s.cacheTabs(wid, tabs)
	return cloneTabs(tabs), nil
}

func (s *CachedStore) GetTab(ctx context.Context, workspaceID, tabID string) (Tab, bool, error) {
	k := tabKey(workspaceID, tabID)
	if t, ok := s.tabByKey.Get(k); ok {
		return t, true, nil
	}
	t, ok, err := s.origin.GetTab(ctx, workspaceID, tabID)
	if err != nil || !ok {
		return t, ok, err
	}
	s.tabByKey.Set(k, t, 1)
	s.tabToWorkspace.Set(strings.TrimSpace(t.TabID), strings.TrimSpace(t.WorkspaceID), 1)
	return t, true, nil
}

func (s *CachedStore) CreateTab(ctx context.Context, workspaceID, title string) (Tab, error) {
	tab, err := s.origin.CreateTab(ctx, workspaceID, title)
	if err != nil {
		return Tab{}, err
	}
	wid := strings.TrimSpace(workspaceID)
	s.tabsByWorkspace.Delete(wid)
	s.tabByKey.Set(tabKey(wid, tab.TabID), tab, 1)
	s.tabToWorkspace.Set(strings.TrimSpace(tab.TabID), wid, 1)
	s.workspaceByID.Delete(wid)
	return tab, nil
}

func (s *CachedStore) SelectTab(ctx context.Context, workspaceID, tabID string) error {
	if err := s.origin.SelectTab(ctx, workspaceID, tabID); err != nil {
		return err
	}
	wid := strings.TrimSpace(workspaceID)
	s.workspaceByID.Delete(wid)
	s.tabsByWorkspace.Delete(wid)
	s.tabByKey.Delete(tabKey(wid, tabID))
	return nil
}

func (s *CachedStore) UpdateTabRun(ctx context.Context, tabID, runID string) error {
	if err := s.origin.UpdateTabRun(ctx, tabID, runID); err != nil {
		return err
	}
	tid := strings.TrimSpace(tabID)
	if wid, ok := s.tabToWorkspace.Get(tid); ok {
		s.tabsByWorkspace.Delete(wid)
		s.tabByKey.Delete(tabKey(wid, tid))
	}
	return nil
}

func (s *CachedStore) cacheWorkspace(ws Workspace) {
	pid := strings.TrimSpace(ws.ProjectID)
	wid := strings.TrimSpace(ws.WorkspaceID)
	if pid != "" {
		s.workspaceByProject.Set(pid, ws, 1)
	}
	if wid != "" {
		s.workspaceByID.Set(wid, ws, 1)
	}
}

func (s *CachedStore) cacheTabs(workspaceID string, tabs []Tab) {
	copied := cloneTabs(tabs)
	s.tabsByWorkspace.Set(workspaceID, copied, len(copied))
	for _, t := range copied {
		s.tabByKey.Set(tabKey(workspaceID, t.TabID), t, 1)
		s.tabToWorkspace.Set(strings.TrimSpace(t.TabID), workspaceID, 1)
	}
}

func cloneTabs(in []Tab) []Tab {
	if len(in) == 0 {
		return nil
	}
	out := make([]Tab, len(in))
	copy(out, in)
	return out
}

func tabKey(workspaceID, tabID string) string {
	return strings.TrimSpace(workspaceID) + "|" + strings.TrimSpace(tabID)
}
