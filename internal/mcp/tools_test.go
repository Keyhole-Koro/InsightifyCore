package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"insightify/internal/snippet"
	"insightify/internal/safeio"
	"insightify/internal/scan"
	cb "insightify/internal/types/codebase"
)

func setupRepo(t *testing.T) (repoRoot string, repoFS *safeio.SafeFS, artifactFS *safeio.SafeFS) {
	t.Helper()
	base := t.TempDir()
	scan.SetReposDir(base)
	repoRoot = filepath.Join(base, "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	var err error
	repoFS, err = safeio.NewSafeFS(repoRoot)
	if err != nil {
		t.Fatalf("repo fs: %v", err)
	}
	scan.SetSafeFS(repoFS)

	artDir := filepath.Join(base, "artifacts")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatalf("mkdir artifacts: %v", err)
	}
	artifactFS, err = safeio.NewSafeFS(artDir)
	if err != nil {
		t.Fatalf("artifact fs: %v", err)
	}
	return repoRoot, repoFS, artifactFS
}

func TestScanListTool(t *testing.T) {
	repoRoot, repoFS, _ := setupRepo(t)
	_ = repoFS

	if err := os.WriteFile(filepath.Join(repoRoot, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "sub", "b.go"), []byte("package sub"), 0o644); err != nil {
		t.Fatalf("write b.go: %v", err)
	}

	tool := newScanListTool(Host{RepoRoot: repoRoot, RepoFS: repoFS})
	in := scanListInput{Roots: []string{"."}, AllowExt: []string{".txt", ".go"}, MaxDepth: 3}
	raw, _ := json.Marshal(in)
	outRaw, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("scan.list call: %v", err)
	}
	var out scanListOutput
	if err := json.Unmarshal(outRaw, &out); err != nil {
		t.Fatalf("scan.list decode: %v", err)
	}
	if len(out.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(out.Files))
	}
	paths := map[string]struct{}{}
	for _, f := range out.Files {
		paths[f.Path] = struct{}{}
	}
	if _, ok := paths["a.txt"]; !ok {
		t.Fatalf("missing a.txt in scan.list output")
	}
	if _, ok := paths[filepath.ToSlash("sub/b.go")]; !ok {
		t.Fatalf("missing sub/b.go in scan.list output")
	}
}

func TestFSReadTool(t *testing.T) {
	repoRoot, repoFS, _ := setupRepo(t)
	content := "hello world"
	if err := os.WriteFile(filepath.Join(repoRoot, "file.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("write file.txt: %v", err)
	}
	tool := newFSReadTool(Host{RepoRoot: repoRoot, RepoFS: repoFS})
	in := fsReadInput{Path: "file.txt", Start: 6, Length: 5}
	raw, _ := json.Marshal(in)
	outRaw, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("fs.read call: %v", err)
	}
	var out fsReadOutput
	if err := json.Unmarshal(outRaw, &out); err != nil {
		t.Fatalf("fs.read decode: %v", err)
	}
	if out.Content != "world" {
		t.Fatalf("expected 'world', got %q", out.Content)
	}
}

func TestWordIdxSearchTool(t *testing.T) {
	repoRoot, repoFS, _ := setupRepo(t)
	data := "hello world\nhello again\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "a.go"), []byte(data), 0o644); err != nil {
		t.Fatalf("write a.go: %v", err)
	}
	tool := newWordIdxSearchTool(Host{RepoRoot: repoRoot, RepoFS: repoFS})
	in := wordIdxInput{Roots: []string{"."}, Word: "hello", AllowExt: []string{".go"}, MaxResults: 5}
	raw, _ := json.Marshal(in)
	outRaw, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("wordidx.search call: %v", err)
	}
	var out wordIdxOutput
	if err := json.Unmarshal(outRaw, &out); err != nil {
		t.Fatalf("wordidx.search decode: %v", err)
	}
	if len(out.Matches) == 0 {
		t.Fatalf("expected matches, got none")
	}
	found := false
	for _, m := range out.Matches {
		if strings.HasSuffix(filepath.ToSlash(m.Path), "/a.go") || m.Path == "a.go" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected match for a.go, got %+v", out.Matches)
	}
}

func TestSnippetCollectTool(t *testing.T) {
	repoRoot, repoFS, artifactFS := setupRepo(t)
	code := "Foo := 1\nbar := 2\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte(code), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	c4 := cb.C4Out{
		Repo: repoRoot,
		Files: []cb.IdentifierReport{
			{
				Path: "main.go",
				Identifiers: []cb.IdentifierSignal{
					{
						Name:  "Foo",
						Lines: [2]int{1, 1},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(c4)
	if err := os.WriteFile(filepath.Join(artifactFS.Root(), "c4.json"), b, 0o644); err != nil {
		t.Fatalf("write c4.json: %v", err)
	}

	tool := newSnippetCollectTool(Host{RepoRoot: repoRoot, RepoFS: repoFS, ArtifactFS: artifactFS})
	in := snippetCollectInput{Seeds: []snippet.Identifier{{Path: "main.go", Name: "Foo"}}, MaxTokens: 0}
	raw, _ := json.Marshal(in)
	outRaw, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("snippet.collect call: %v", err)
	}
	var out snippetCollectOutput
	if err := json.Unmarshal(outRaw, &out); err != nil {
		t.Fatalf("snippet.collect decode: %v", err)
	}
	if len(out.Snippets) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(out.Snippets))
	}
	if !strings.Contains(out.Snippets[0].Code, "Foo") {
		t.Fatalf("expected snippet to contain Foo, got %q", out.Snippets[0].Code)
	}
}
