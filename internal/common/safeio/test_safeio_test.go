package safeio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeFSAllowsAbsoluteUnderRoot(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	fs, err := NewSafeFS(dir)
	if err != nil {
		t.Fatalf("NewSafeFS: %v", err)
	}
	if _, err := fs.SafeReadFile(p); err != nil {
		t.Fatalf("SafeReadFile absolute: %v", err)
	}
}
