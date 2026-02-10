package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type persistedSession struct {
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
	RepoURL   string `json:"repo_url"`
	Purpose   string `json:"purpose"`
	Repo      string `json:"repo"`
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
			initRunStore.sessions[row.SessionID] = initSession{
				SessionID: row.SessionID,
				UserID:    row.UserID,
				RepoURL:   row.RepoURL,
				Purpose:   row.Purpose,
				Repo:      row.Repo,
				RunCtx:    nil, // lazily recreated when needed
				Running:   false,
			}
		}
	})
}

func persistSessionStore() {
	initRunStore.RLock()
	rows := make([]persistedSession, 0, len(initRunStore.sessions))
	for sid, sess := range initRunStore.sessions {
		rows = append(rows, persistedSession{
			SessionID: sid,
			UserID:    sess.UserID,
			RepoURL:   sess.RepoURL,
			Purpose:   sess.Purpose,
			Repo:      sess.Repo,
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
	initRunStore.sessions[sessionID] = sess
	return sess, true
}

func ensureSessionRunContext(sessionID string) (initSession, error) {
	sess, ok := getSession(sessionID)
	if !ok {
		return initSession{}, fmt.Errorf("session %s not found", sessionID)
	}
	if sess.RunCtx != nil {
		return sess, nil
	}

	runCtx, err := NewRunContext(sess.Repo, sessionID)
	if err != nil {
		return initSession{}, fmt.Errorf("failed to restore run context: %w", err)
	}
	if runCtx != nil && runCtx.Env != nil {
		runCtx.Env.InitPurpose = strings.TrimSpace(sess.Purpose)
		runCtx.Env.InitPurposeRepoURL = strings.TrimSpace(sess.RepoURL)
	}

	updated, _ := updateSession(sessionID, func(cur *initSession) {
		cur.RunCtx = runCtx
	})
	return updated, nil
}
