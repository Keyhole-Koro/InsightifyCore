package runtime

import "strings"

func (a *App) EnsureProjectStoreLoaded() {
	if a == nil || a.projectStore == nil {
		return
	}
	a.projectStore.EnsureLoaded()
}

func (a *App) PersistProjectStore() {
	if a == nil || a.projectStore == nil {
		return
	}
	a.projectStore.Save()
}

func (a *App) GetProject(projectID string) (Project, bool) {
	if a == nil || a.projectStore == nil {
		return Project{}, false
	}
	state, ok := a.projectStore.Get(projectID)
	if !ok {
		return Project{}, false
	}
	a.runCtxMu.RLock()
	ctx := a.runCtx[strings.TrimSpace(projectID)]
	a.runCtxMu.RUnlock()
	return Project{State: state, RunCtx: ctx}, true
}

func (a *App) PutProject(p Project) {
	if a == nil || a.projectStore == nil || strings.TrimSpace(p.State.ProjectID) == "" {
		return
	}
	a.projectStore.Put(p.State)
	a.runCtxMu.Lock()
	a.runCtx[p.State.ProjectID] = p.RunCtx
	a.runCtxMu.Unlock()
}

func (a *App) ListProjectsByUser(userID string) []Project {
	if a == nil || a.projectStore == nil {
		return nil
	}
	states := a.projectStore.ListByUser(userID)
	out := make([]Project, 0, len(states))
	a.runCtxMu.RLock()
	for _, st := range states {
		out = append(out, Project{State: st, RunCtx: a.runCtx[st.ProjectID]})
	}
	a.runCtxMu.RUnlock()
	return out
}

func (a *App) GetActiveProjectByUser(userID string) (Project, bool) {
	if a == nil || a.projectStore == nil {
		return Project{}, false
	}
	st, ok := a.projectStore.GetActiveByUser(userID)
	if !ok {
		return Project{}, false
	}
	a.runCtxMu.RLock()
	ctx := a.runCtx[st.ProjectID]
	a.runCtxMu.RUnlock()
	return Project{State: st, RunCtx: ctx}, true
}

func (a *App) SetActiveProjectForUser(userID, projectID string) (Project, bool) {
	if a == nil || a.projectStore == nil {
		return Project{}, false
	}
	st, ok := a.projectStore.SetActiveForUser(userID, projectID)
	if !ok {
		return Project{}, false
	}
	a.runCtxMu.RLock()
	ctx := a.runCtx[st.ProjectID]
	a.runCtxMu.RUnlock()
	return Project{State: st, RunCtx: ctx}, true
}
