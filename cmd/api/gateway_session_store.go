package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func isProjectID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), "project-")
}

type persistedSession struct {
	SessionID   string `json:"session_id"`
	ProjectID   string `json:"project_id,omitempty"`
	ProjectName string `json:"project_name,omitempty"`
	UserID      string `json:"user_id"`
	RepoURL     string `json:"repo_url"`
	Purpose     string `json:"purpose"`
	Repo        string `json:"repo"`
	IsActive    bool   `json:"is_active,omitempty"`
}

var sessionStoreLoadOnce sync.Once

func sessionStorePath() string {
	return filepath.Join("tmp", "init_sessions.json")
}

func ensureSessionStoreLoaded() {
	sessionStoreLoadOnce.Do(func() {
		data, err := os.ReadFile(sessionStorePath())
		if err != nil {
			return
		}
		var rows []persistedSession
		if err := json.Unmarshal(data, &rows); err != nil {
			return
		}
		initRunStore.Lock()
		defer initRunStore.Unlock()
		for _, row := range rows {
			if row.SessionID == "" {
				continue
			}
			projectID := strings.TrimSpace(row.ProjectID)
			if projectID == "" {
				projectID = row.SessionID
			}
			projectName := strings.TrimSpace(row.ProjectName)
			if projectName == "" {
				projectName = "Project"
			}
			initRunStore.sessions[row.SessionID] = initSession{
				SessionID:   row.SessionID,
				ProjectID:   projectID,
				ProjectName: projectName,
				UserID:      row.UserID,
				RepoURL:     row.RepoURL,
				Purpose:     row.Purpose,
				Repo:        row.Repo,
				IsActive:    row.IsActive,
				RunCtx:      nil, // lazily recreated when needed
				Running:     false,
			}
		}
	})
}

func persistSessionStore() {
	initRunStore.RLock()
	rows := make([]persistedSession, 0, len(initRunStore.sessions))
	for sid, sess := range initRunStore.sessions {
		rows = append(rows, persistedSession{
			SessionID:   sid,
			ProjectID:   sess.ProjectID,
			ProjectName: sess.ProjectName,
			UserID:      sess.UserID,
			RepoURL:     sess.RepoURL,
			Purpose:     sess.Purpose,
			Repo:        sess.Repo,
			IsActive:    sess.IsActive,
		})
	}
	initRunStore.RUnlock()

	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return
	}
	p := sessionStorePath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, data, 0o644)
}

func getSession(sessionID string) (initSession, bool) {
	initRunStore.RLock()
	sess, ok := initRunStore.sessions[sessionID]
	initRunStore.RUnlock()
	return sess, ok
}

func putSession(sess initSession) {
	if strings.TrimSpace(sess.SessionID) == "" {
		return
	}
	if strings.TrimSpace(sess.ProjectID) == "" {
		sess.ProjectID = sess.SessionID
	}
	if strings.TrimSpace(sess.ProjectName) == "" {
		sess.ProjectName = "Project"
	}
	initRunStore.Lock()
	initRunStore.sessions[sess.SessionID] = sess
	initRunStore.Unlock()
}

func updateSession(sessionID string, update func(*initSession)) (initSession, bool) {
	initRunStore.Lock()
	defer initRunStore.Unlock()
	sess, ok := initRunStore.sessions[sessionID]
	if !ok {
		return initSession{}, false
	}
	update(&sess)
	sess.SessionID = sessionID
	if strings.TrimSpace(sess.ProjectID) == "" {
		sess.ProjectID = sessionID
	}
	if strings.TrimSpace(sess.ProjectName) == "" {
		sess.ProjectName = "Project"
	}
	initRunStore.sessions[sessionID] = sess
	return sess, true
}

func listProjectsByUser(userID string) []initSession {
	userID = strings.TrimSpace(userID)
	initRunStore.RLock()
	defer initRunStore.RUnlock()
	projects := make([]initSession, 0, len(initRunStore.sessions))
	for _, sess := range initRunStore.sessions {
		if !isProjectID(sess.ProjectID) {
			continue
		}
		if userID != "" && strings.TrimSpace(sess.UserID) != userID {
			continue
		}
		projects = append(projects, sess)
	}
	return projects
}

func getActiveProjectByUser(userID string) (initSession, bool) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return initSession{}, false
	}
	initRunStore.RLock()
	defer initRunStore.RUnlock()
	for _, sess := range initRunStore.sessions {
		if !isProjectID(sess.ProjectID) {
			continue
		}
		if strings.TrimSpace(sess.UserID) != userID {
			continue
		}
		if sess.IsActive {
			return sess, true
		}
	}
	return initSession{}, false
}

func setActiveProjectForUser(userID, projectID string) (initSession, bool) {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	if userID == "" || projectID == "" {
		return initSession{}, false
	}

	initRunStore.Lock()
	defer initRunStore.Unlock()
	var selected initSession
	var found bool
	for key, sess := range initRunStore.sessions {
		if !isProjectID(sess.ProjectID) {
			continue
		}
		if strings.TrimSpace(sess.UserID) != userID {
			continue
		}
		if strings.TrimSpace(sess.ProjectID) == projectID || key == projectID {
			sess.IsActive = true
			selected = sess
			found = true
		} else {
			sess.IsActive = false
		}
		initRunStore.sessions[key] = sess
	}
	return selected, found
}

func ensureSessionRunContext(sessionID string) (initSession, error) {
	sess, ok := getSession(sessionID)
	if !ok {
		return initSession{}, fmt.Errorf("session %s not found", sessionID)
	}
	if sess.RunCtx != nil && hasRequiredWorkers(sess.RunCtx) {
		return sess, nil
	}

	runCtx, err := NewRunContext(sess.Repo, sessionID)
	if err != nil {
		return initSession{}, fmt.Errorf("failed to restore run context: %w", err)
	}
	if runCtx != nil && runCtx.Env != nil {
		runCtx.Env.InitCtx.SetPurpose(sess.Purpose, sess.RepoURL)
	}

	updated, _ := updateSession(sessionID, func(cur *initSession) {
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
