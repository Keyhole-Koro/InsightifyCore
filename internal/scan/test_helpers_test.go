package scan

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"insightify/internal/safeio"
)

var testReposMu sync.Mutex

func setupTestReposDir(t *testing.T) string {
	t.Helper()
	testReposMu.Lock()
	repos := t.TempDir()
	prevFS := CurrentSafeFS()
	fs, err := safeio.NewSafeFS(repos)
	if err != nil {
		t.Fatalf("safe fs: %v", err)
	}
	prev := ReposDir()
	SetReposDir(repos)
	SetSafeFS(fs)
	t.Cleanup(func() {
		SetSafeFS(prevFS)
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

func setSafeFSForTest(t *testing.T, root string) {
	t.Helper()
	prev := CurrentSafeFS()
	fs, err := safeio.NewSafeFS(root)
	if err != nil {
		t.Fatalf("safe fs: %v", err)
	}
	SetSafeFS(fs)
	t.Cleanup(func() { SetSafeFS(prev) })
}
