package scan

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
)

func write(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestStream_FilesOnly(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "root file")
	write(t, root, "dir1/b.txt", "child file")
	write(t, root, "dir1/vendor/skip.txt", "ignored vendor")
	write(t, root, "node_modules/x.txt", "ignored nm")
	write(t, root, "deep/level2/c.txt", "deep file")

	opts := Options{
		IgnoreDirs:  []string{"node_modules", "vendor"},
		BypassCache: true,
	}

	ch, errCh := Stream(root, opts, true)
	var got []string
	for fv := range ch {
		if fv.IsDir {
			t.Fatalf("IsDir came even though filesOnly=true: %+v", fv)
		}
		got = append(got, fv.Path)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("scan error: %v", err)
	}

	sort.Strings(got)
	want := []string{
		"a.txt",
		"deep/level2/c.txt",
		"dir1/b.txt",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
}

func TestScan_IgnoresAndDepth(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "root file")
	write(t, root, "dir1/b.txt", "child file")
	write(t, root, "dir1/c.txt", "child file2")
	write(t, root, "dir1/vendor/skip.txt", "ignored vendor")
	write(t, root, "d/e/f.txt", "deep")

	opts := Options{
		MaxDepth:    1,
		IgnoreDirs:  []string{"vendor"},
		BypassCache: true,
	}

	var mu = make(chan struct{}, 1)
	var files []string

	cb := func(fv FileVisit) {
		if fv.IsDir {
			return
		}
		mu <- struct{}{}
		files = append(files, fv.Path)
		<-mu
	}
	if err := ScanWithOptions(root, opts, cb); err != nil {
		t.Fatalf("scan: %v", err)
	}

	sort.Strings(files)
	want := []string{"a.txt"}
	if !slices.Equal(files, want) {
		t.Fatalf("files=%v want=%v", files, want)
	}
}
