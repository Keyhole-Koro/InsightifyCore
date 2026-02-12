package main

import (
	"fmt"
	"strings"
)

func isProjectID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), "project-")
}

func ensureProjectStoreLoaded() {
	projectStateStore.EnsureLoaded()
}

func persistProjectStore() {
	projectStateStore.Save()
}

func getProjectState(projectID string) (projectState, bool) {
	state, ok := projectStateStore.Get(projectID)
	if !ok {
		return projectState{}, false
	}
	runContextStore.RLock()
	runCtx := runContextStore.byProjectID[strings.TrimSpace(projectID)]
	runContextStore.RUnlock()
	return projectState{State: state, RunCtx: runCtx}, true
}

func putProjectState(sess projectState) {
	if strings.TrimSpace(sess.ProjectID) == "" {
		return
	}
	projectStateStore.Put(sess.State)
	runContextStore.Lock()
	runContextStore.byProjectID[sess.ProjectID] = sess.RunCtx
	runContextStore.Unlock()
}

func updateProjectState(projectID string, update func(*projectState)) (projectState, bool) {
	current, ok := getProjectState(projectID)
	if !ok {
		return projectState{}, false
	}
	update(&current)
	projectStateStore.Put(current.State)
	runContextStore.Lock()
	runContextStore.byProjectID[current.ProjectID] = current.RunCtx
	runContextStore.Unlock()
	return current, true
}

func listProjectsByUser(userID string) []projectState {
	states := projectStateStore.ListByUser(userID)
	projects := make([]projectState, 0, len(states))
	runContextStore.RLock()
	for _, state := range states {
		if !isProjectID(state.ProjectID) {
			continue
		}
		projects = append(projects, projectState{
			State:  state,
			RunCtx: runContextStore.byProjectID[state.ProjectID],
		})
	}
	runContextStore.RUnlock()
	return projects
}

func getActiveProjectByUser(userID string) (projectState, bool) {
	for _, project := range listProjectsByUser(userID) {
		if project.IsActive {
			return project, true
		}
	}
	return projectState{}, false
}

func setActiveProjectForUser(userID, projectID string) (projectState, bool) {
	state, ok := projectStateStore.SetActiveForUser(userID, projectID)
	if !ok {
		return projectState{}, false
	}
	if !isProjectID(state.ProjectID) {
		return projectState{}, false
	}
	runContextStore.RLock()
	runCtx := runContextStore.byProjectID[state.ProjectID]
	runContextStore.RUnlock()
	return projectState{State: state, RunCtx: runCtx}, true
}

func ensureProjectRunContext(projectID string) (projectState, error) {
	sess, ok := getProjectState(projectID)
	if !ok {
		return projectState{}, fmt.Errorf("project %s not found", projectID)
	}
	if sess.RunCtx != nil && hasRequiredWorkers(sess.RunCtx) {
		return sess, nil
	}

	runCtx, err := NewRunContext(sess.Repo, projectID)
	if err != nil {
		return projectState{}, fmt.Errorf("failed to restore run context: %w", err)
	}

	updated, _ := updateProjectState(projectID, func(cur *projectState) {
		cur.RunCtx = runCtx
	})
	return updated, nil
}

func hasRequiredWorkers(runCtx *RunContext) bool {
	if runCtx == nil || runCtx.Env == nil || runCtx.Env.Resolver == nil {
		return false
	}
	_, hasBootstrap := runCtx.Env.Resolver.Get("bootstrap")
	_, hasTestLLM := runCtx.Env.Resolver.Get("testllmChar")
	return hasBootstrap && hasTestLLM
}
