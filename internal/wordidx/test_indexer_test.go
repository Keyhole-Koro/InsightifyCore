package wordidx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"insightify/internal/safeio"
	"insightify/internal/scan"
)

func setupWordidxRepos(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	prev := scan.ReposDir()
	scan.SetReposDir(base)
	prevFS := scan.CurrentSafeFS()
	fs, err := safeio.NewSafeFS(base)
	if err != nil {
		t.Fatalf("safe fs: %v", err)
	}
	scan.SetSafeFS(fs)
	t.Cleanup(func() {
		scan.SetSafeFS(prevFS)
		scan.SetReposDir(prev)
	})
	return base
}

func mkrepo(t *testing.T, base string) string {
	return mkNamedRepo(t, base, "repo")
}

func mkNamedRepo(t *testing.T, base, name string) string {
	t.Helper()
	root := filepath.Join(base, name)
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
	base := setupWordidxRepos(t)
	root := mkrepo(t, base)

	agg := New().
		Root(root).
		Allow("txt").
		Start(context.Background())

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
	base := setupWordidxRepos(t)
	root := mkrepo(t, base)
	agg := New().
		Root(root).
		Allow("bin").
		Workers(2).
		Options(scan.Options{BypassCache: true}).
		Start(context.Background()) // bin のみ

	if err := agg.Wait(context.Background()); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if got := agg.Find(context.Background(), "world"); len(got) != 0 {
		t.Fatalf("unexpected hits for 'world' with bin-only filter: %v", got)
	}
}

func TestAggIndex_FilesSnapshot(t *testing.T) {
	base := setupWordidxRepos(t)
	root := mkrepo(t, base)

	agg := New().
		Root(root).
		Allow("txt").
		Start(context.Background())

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

func TestBuilder_MultiRoots(t *testing.T) {
	base := setupWordidxRepos(t)
	root1 := mkrepo(t, base)
	root2 := filepath.Join(base, "repo2")
	if err := os.MkdirAll(root2, 0o755); err != nil {
		t.Fatalf("mkdir repo2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root2, "c.txt"), []byte("world from second root"), 0o644); err != nil {
		t.Fatalf("write root2: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agg := New().
		Root(root1, root2).
		Allow("txt").
		Start(ctx)

	refs := agg.Find(ctx, "world")
	if len(refs) == 0 {
		t.Fatalf("expected hits for 'world'")
	}
	var sawSecond bool
	for _, r := range refs {
		if strings.EqualFold(filepath.Base(r.FilePath), "c.txt") {
			sawSecond = true
			break
		}
	}
	if !sawSecond {
		t.Fatalf("expected to see c.txt from second root, refs=%v", refs)
	}
}
