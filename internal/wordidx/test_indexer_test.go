package wordidx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"insightify/internal/scan"
)

func mkrepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mk := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	mk("a.txt", `
hello world
_foo bar baz
123 "quote" world
`)
	mk("nested/b.txt", `
alpha _id
world hello
`)
	mk("skip.bin", "\x00\x01\x02 binary should be filtered")

	return root
}

func TestBuild_Simple(t *testing.T) {
	src := []byte(`_x  hello 123 world "q" foo_bar`)
	idx := Build(src)

	// present
	for _, w := range []string{"_x", "hello", "world", "foo_bar"} {
		if ps := idx.Find(w); len(ps) == 0 {
			t.Fatalf("expected to find %q", w)
		}
	}
	// absent (numbers / case)
	for _, w := range []string{"123", "Hello"} {
		if ps := idx.Find(w); len(ps) != 0 {
			t.Fatalf("did not expect to find %q", w)
		}
	}
}

func TestAggIndex_StartFromScan_FindBlocksUntilDone(t *testing.T) {
	root := mkrepo(t)

	agg := NewAgg()
	sopts := scan.Options{
		IgnoreDirs:  []string{".git", "node_modules", "vendor"},
		BypassCache: true,
	}
	agg.StartFromScan(context.Background(), root, sopts, 0, ExtAllow("txt"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	refs := agg.Find(ctx, "world")
	if len(refs) == 0 {
		t.Fatalf("expected hits for 'world'")
	}

	seen := map[string]bool{}
	for _, r := range refs {
		seen[filepath.Base(r.FilePath)] = true
	}
	if !(seen["a.txt"] && seen["b.txt"]) {
		t.Fatalf("expected hits in a.txt and b.txt, got=%v", seen)
	}
}

func TestAggIndex_FilterExtensions(t *testing.T) {
	root := mkrepo(t)

	agg := NewAgg()
	sopts := scan.Options{BypassCache: true}
	agg.StartFromScan(context.Background(), root, sopts, 2, ExtAllow("bin")) // bin のみ

	if err := agg.Wait(context.Background()); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if got := agg.Find(context.Background(), "world"); len(got) != 0 {
		t.Fatalf("unexpected hits for 'world' with bin-only filter: %v", got)
	}
}

func TestAggIndex_FilesSnapshot(t *testing.T) {
	root := mkrepo(t)

	agg := NewAgg()
	agg.StartFromScan(context.Background(), root, scan.Options{BypassCache: true}, 0, ExtAllow("txt"))

	if err := agg.Wait(context.Background()); err != nil {
		t.Fatalf("wait: %v", err)
	}
	files := agg.Files(context.Background())
	if len(files) == 0 {
		t.Fatalf("expected some file indices")
	}
	// all indexed files must be .txt as we used ExtAllow("txt")
	for _, fi := range files {
		if !strings.EqualFold(filepath.Ext(fi.Path), ".txt") {
			t.Fatalf("unexpected file in snapshot: %s", fi.Path)
		}
		if fi.Index == nil {
			t.Fatalf("nil per-file index for %s", fi.Path)
		}
	}
}
