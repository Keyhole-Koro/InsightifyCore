package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"insightify/internal/safeio"
	"insightify/internal/scan"
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
	in := scanListInput{Roots: []string{"."}, AllowExt: []string{"iteritems.txt", ".go"}, MaxDepth: 3}
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

func TestDeltaDiffTool(t *testing.T) {
	tool := newDeltaDiffTool()
	before := map[string]any{
		"architecture_hypothesis": map[string]any{
			"purpose": "old",
			"key_components": []any{
				map[string]any{"name": "API"},
			},
		},
	}
	after := map[string]any{
		"architecture_hypothesis": map[string]any{
			"purpose": "new",
			"key_components": []any{
				map[string]any{"name": "API"},
				map[string]any{"name": "Worker"},
			},
		},
		"notes": []any{"extra"},
	}
	in := deltaDiffInput{}
	if b, err := json.Marshal(before); err == nil {
		in.Before = b
	}
	if b, err := json.Marshal(after); err == nil {
		in.After = b
	}
	raw, _ := json.Marshal(in)
	outRaw, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("delta.diff call: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(outRaw, &out); err != nil {
		t.Fatalf("delta.diff decode: %v", err)
	}
	mods, ok := out["modified"].([]any)
	if !ok || len(mods) == 0 {
		t.Fatalf("expected modified entries, got %+v", out["modified"])
	}
}

func TestGitHubCloneTool_CloneAndValidation(t *testing.T) {
	repoRoot, _, _ := setupRepo(t)
	reposRoot := filepath.Dir(repoRoot)
	tool := newGitHubCloneTool(Host{ReposRoot: reposRoot})

	orig := runGitCommand
	t.Cleanup(func() { runGitCommand = orig })
	var calls [][]string
	runGitCommand = func(ctx context.Context, args ...string) error {
		_ = ctx
		calls = append(calls, append([]string{}, args...))
		return nil
	}

	raw, _ := json.Marshal(githubCloneInput{
		RepoURL: "https://github.com/Keyhole-Koro/PoliTopics.git",
	})
	outRaw, err := tool.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("github.clone call: %v", err)
	}
	var out githubCloneOutput
	if err := json.Unmarshal(outRaw, &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out.Status != "cloned" || out.RepoName != "PoliTopics" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 git call, got %d", len(calls))
	}
	args := strings.Join(calls[0], " ")
	if !strings.Contains(args, "clone --depth 1 https://github.com/Keyhole-Koro/PoliTopics.git") {
		t.Fatalf("unexpected clone args: %v", calls[0])
	}

	badRaw, _ := json.Marshal(githubCloneInput{RepoURL: "https://gitlab.com/example/hello"})
	if _, err := tool.Call(context.Background(), badRaw); err == nil {
		t.Fatalf("expected non-github URL to fail")
	}
}

func TestGitHubCloneTool_IfExistsSkipAndPull(t *testing.T) {
	repoRoot, _, _ := setupRepo(t)
	reposRoot := filepath.Dir(repoRoot)
	target := filepath.Join(reposRoot, "already")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	tool := newGitHubCloneTool(Host{ReposRoot: reposRoot})

	orig := runGitCommand
	t.Cleanup(func() { runGitCommand = orig })
	var calls [][]string
	runGitCommand = func(ctx context.Context, args ...string) error {
		_ = ctx
		calls = append(calls, append([]string{}, args...))
		return nil
	}

	skipRaw, _ := json.Marshal(githubCloneInput{
		RepoURL:    "git@github.com:Keyhole-Koro/PoliTopics.git",
		TargetName: "already",
		IfExists:   "skip",
	})
	outRaw, err := tool.Call(context.Background(), skipRaw)
	if err != nil {
		t.Fatalf("skip call: %v", err)
	}
	var out githubCloneOutput
	if err := json.Unmarshal(outRaw, &out); err != nil {
		t.Fatalf("decode skip output: %v", err)
	}
	if out.Status != "skipped" {
		t.Fatalf("unexpected skip status: %+v", out)
	}
	if len(calls) != 0 {
		t.Fatalf("skip should not call git: %v", calls)
	}

	pullRaw, _ := json.Marshal(githubCloneInput{
		RepoURL:    "git@github.com:Keyhole-Koro/PoliTopics.git",
		TargetName: "already",
		IfExists:   "pull",
		Branch:     "main",
	})
	outRaw, err = tool.Call(context.Background(), pullRaw)
	if err != nil {
		t.Fatalf("pull call: %v", err)
	}
	if err := json.Unmarshal(outRaw, &out); err != nil {
		t.Fatalf("decode pull output: %v", err)
	}
	if out.Status != "updated" {
		t.Fatalf("unexpected pull status: %+v", out)
	}
	if len(calls) != 1 {
		t.Fatalf("pull should call git once, got %d", len(calls))
	}
	wantPrefix := []string{"-C", target, "pull", "--ff-only", "origin", "main"}
	for i, w := range wantPrefix {
		if i >= len(calls[0]) || calls[0][i] != w {
			t.Fatalf("unexpected pull args: %v", calls[0])
		}
	}
}
