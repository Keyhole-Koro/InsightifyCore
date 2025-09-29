package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileInfo_Basic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry, preview, err := FileInfo(dir, "a.txt", 5)
	if err != nil {
		t.Fatalf("FileInfo: %v", err)
	}
	if entry.Path != "a.txt" {
		t.Fatalf("path mismatch: %q", entry.Path)
	}
	if entry.Size <= 0 {
		t.Fatalf("size should be > 0")
	}
	if preview != "hello" {
		t.Fatalf("preview mismatch: %q", preview)
	}

	// No content when limit == 0
	_, preview2, err := FileInfo(dir, "a.txt", 0)
	if err != nil {
		t.Fatalf("FileInfo(0): %v", err)
	}
	if preview2 != "" {
		t.Fatalf("expected empty preview when limit=0")
	}
}

func TestFileInfo_DirectoryError(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := FileInfo(dir, ".", 10); err == nil {
		t.Fatalf("expected error for directory path")
	}
}
