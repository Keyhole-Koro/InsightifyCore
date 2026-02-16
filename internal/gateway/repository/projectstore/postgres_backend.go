package projectstore

import (
	"database/sql"
	"strings"
)

func (s *Store) ensureSchema() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.schemaOnce.Do(func() {
		_, s.schemaErr = s.db.Exec(`
CREATE TABLE IF NOT EXISTS project_states (
  project_id TEXT PRIMARY KEY,
  project_name TEXT NOT NULL DEFAULT 'Project',
  user_id TEXT NOT NULL DEFAULT '',
  repo TEXT NOT NULL DEFAULT '',
  is_active BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS project_artifacts (
  id SERIAL PRIMARY KEY,
  project_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  path TEXT NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE (run_id, path)
);
CREATE INDEX IF NOT EXISTS idx_project_artifacts_project_id ON project_artifacts (project_id);
CREATE INDEX IF NOT EXISTS idx_project_artifacts_run_id ON project_artifacts (run_id);
`)
	})
	return s.schemaErr
}

func scanStateDB(row rowScanner) (State, bool) {
	var state State
	err := row.Scan(
		&state.ProjectID,
		&state.ProjectName,
		&state.UserID,
		&state.Repo,
		&state.IsActive,
	)
	if err != nil {
		return State{}, false
	}
	return normalizeState(state), true
}

func (s *Store) getDB(projectID string) (State, bool) {
	if err := s.ensureSchema(); err != nil {
		return State{}, false
	}
	id := strings.TrimSpace(projectID)
	if id == "" {
		return State{}, false
	}
	row := s.db.QueryRow(`SELECT project_id, project_name, user_id, repo, is_active
FROM project_states WHERE project_id = $1`, id)
	return scanStateDB(row)
}

func (s *Store) putDB(state State) {
	if err := s.ensureSchema(); err != nil {
		return
	}
	n := normalizeState(state)
	if n.ProjectID == "" {
		return
	}
	_, _ = s.db.Exec(`
INSERT INTO project_states (
  project_id, project_name, user_id, repo, is_active
)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (project_id)
DO UPDATE SET project_name=EXCLUDED.project_name,
  user_id=EXCLUDED.user_id,
  repo=EXCLUDED.repo,
  is_active=EXCLUDED.is_active`,
		n.ProjectID, n.ProjectName, n.UserID, n.Repo, n.IsActive)
}

func (s *Store) updateDB(projectID string, update func(*State)) (State, bool) {
	if err := s.ensureSchema(); err != nil {
		return State{}, false
	}
	tx, err := s.db.Begin()
	if err != nil {
		return State{}, false
	}
	defer func() { _ = tx.Rollback() }()

	id := strings.TrimSpace(projectID)
	row := tx.QueryRow(`SELECT project_id, project_name, user_id, repo, is_active
FROM project_states WHERE project_id = $1 FOR UPDATE`, id)
	cur, ok := scanStateDB(row)
	if !ok {
		return State{}, false
	}
	update(&cur)
	cur.ProjectID = id
	cur = normalizeState(cur)
	_, err = tx.Exec(`
UPDATE project_states
SET project_name=$2, user_id=$3, repo=$4, is_active=$5
WHERE project_id=$1`,
		cur.ProjectID, cur.ProjectName, cur.UserID, cur.Repo, cur.IsActive)
	if err != nil {
		return State{}, false
	}
	if err := tx.Commit(); err != nil {
		return State{}, false
	}
	return cur, true
}

func (s *Store) listByUserDB(userID string) []State {
	if err := s.ensureSchema(); err != nil {
		return nil
	}
	uid := strings.TrimSpace(userID)
	var (
		rows *sql.Rows
		err  error
	)
	if uid == "" {
		rows, err = s.db.Query(`SELECT project_id, project_name, user_id, repo, is_active
FROM project_states`)
	} else {
		rows, err = s.db.Query(`SELECT project_id, project_name, user_id, repo, is_active
FROM project_states WHERE user_id = $1`, uid)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]State, 0, 32)
	for rows.Next() {
		var state State
		if err := rows.Scan(&state.ProjectID, &state.ProjectName, &state.UserID, &state.Repo, &state.IsActive); err != nil {
			continue
		}
		out = append(out, normalizeState(state))
	}
	return out
}

func (s *Store) getActiveByUserDB(userID string) (State, bool) {
	if err := s.ensureSchema(); err != nil {
		return State{}, false
	}
	uid := strings.TrimSpace(userID)
	if uid == "" {
		return State{}, false
	}
	row := s.db.QueryRow(`SELECT project_id, project_name, user_id, repo, is_active
FROM project_states WHERE user_id = $1 AND is_active = TRUE LIMIT 1`, uid)
	return scanStateDB(row)
}

func (s *Store) setActiveForUserDB(userID, projectID string) (State, bool) {
	if err := s.ensureSchema(); err != nil {
		return State{}, false
	}
	uid := strings.TrimSpace(userID)
	pid := strings.TrimSpace(projectID)
	if uid == "" || pid == "" {
		return State{}, false
	}
	tx, err := s.db.Begin()
	if err != nil {
		return State{}, false
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRow(`SELECT project_id, project_name, user_id, repo, is_active
FROM project_states WHERE project_id = $1 AND user_id = $2 FOR UPDATE`, pid, uid)
	target, ok := scanStateDB(row)
	if !ok {
		return State{}, false
	}
	_, _ = tx.Exec(`UPDATE project_states SET is_active = FALSE WHERE user_id = $1 AND project_id <> $2`, uid, pid)
	_, err = tx.Exec(`UPDATE project_states SET is_active = TRUE WHERE project_id = $1`, pid)
	if err != nil {
		return State{}, false
	}
	target.IsActive = true
	target = normalizeState(target)
	if err := tx.Commit(); err != nil {
		return State{}, false
	}
	return target, true
}

func (s *Store) addArtifactDB(artifact ProjectArtifact) error {
	if err := s.ensureSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`
INSERT INTO project_artifacts (project_id, run_id, path, created_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (run_id, path) DO NOTHING`,
		artifact.ProjectID, artifact.RunID, artifact.Path)
	return err
}

func (s *Store) listArtifactsDB(projectID string) ([]ProjectArtifact, error) {
	if err := s.ensureSchema(); err != nil {
		return nil, err
	}
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return nil, nil
	}
	rows, err := s.db.Query(`
SELECT id, project_id, run_id, path, created_at
FROM project_artifacts
WHERE project_id = $1
ORDER BY created_at DESC`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProjectArtifact
	for rows.Next() {
		var a ProjectArtifact
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.RunID, &a.Path, &a.CreatedAt); err != nil {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}
