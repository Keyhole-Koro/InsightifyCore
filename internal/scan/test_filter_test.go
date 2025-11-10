package scan

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestFilesWithExtensions(t *testing.T) {
	root := t.TempDir()

	mustWrite := func(rel string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte("dummy"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	mustWrite("main.go")
	mustWrite("helper.GO")
	mustWrite("README.md")
	mustWrite("lib/util.ts")
	mustWrite("lib/util.go")
	mustWrite("vendor/skip.go")

	opts := Options{IgnoreDirs: []string{"vendor"}}
	files, err := FilesWithExtensions(root, []string{".go", "TS"}, opts)
	if err != nil {
		t.Fatalf("FilesWithExtensions: %v", err)
	}

	want := []string{"helper.GO", "lib/util.go", "lib/util.ts", "main.go"}
	if len(files) != len(want) {
		t.Fatalf("unexpected count: got %d want %d (files=%v)", len(files), len(want), files)
	}

	sort.Strings(files)
	for i := range want {
		if files[i] != want[i] {
			t.Fatalf("files[%d]=%s want %s (all=%v)", i, files[i], want[i], files)
		}
	}
}
