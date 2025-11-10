package codebase

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"insightify/internal/scan"
)

func TestFilesWithExtensions_UsesRepoRoots(t *testing.T) {
	repo := t.TempDir()

	mustWrite := func(rel string) {
		path := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	mustWrite("api/handler.ts")
	mustWrite("api/nested/util.ts")
	mustWrite("api/node_modules/ignore.ts")
	mustWrite("src/main.go")
	mustWrite("src/pkg/util.go")
	mustWrite("README.md")

	opts := scan.Options{IgnoreDirs: []string{"node_modules"}}
	files, err := FilesWithExtensions(repo, []string{"api", "./src"}, []string{".ts", ".GO"}, opts)
	if err != nil {
		t.Fatalf("FilesWithExtensions: %v", err)
	}

	want := []string{"api/handler.ts", "api/nested/util.ts", "src/main.go", "src/pkg/util.go"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("unexpected file list\n got: %v\nwant: %v", files, want)
	}
}

func TestFilesWithExtensions_DedupesAndHandlesAbsoluteRoots(t *testing.T) {
	repo := t.TempDir()

	file := filepath.Join(repo, "src", "main.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	absRoot := filepath.Join(repo, "src")
	files, err := FilesWithExtensions(repo, []string{"src", absRoot}, []string{".go"}, scan.Options{})
	if err != nil {
		t.Fatalf("FilesWithExtensions: %v", err)
	}

	want := []string{"src/main.go"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("unexpected file list\n got: %v\nwant: %v", files, want)
	}
}
