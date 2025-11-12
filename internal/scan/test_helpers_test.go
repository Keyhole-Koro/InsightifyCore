package scan

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var testReposMu sync.Mutex

func setupTestReposDir(t *testing.T) string {
	t.Helper()
	testReposMu.Lock()
	repos := t.TempDir()
	prev := ReposDir()
	SetReposDir(repos)
	t.Cleanup(func() {
		SetReposDir(prev)
		testReposMu.Unlock()
	})
	return repos
}

func ensureRepoDir(t *testing.T, reposDir, name string) string {
	t.Helper()
	root := filepath.Join(reposDir, name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	return root
}
