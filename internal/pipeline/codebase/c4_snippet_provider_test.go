package codebase

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"insightify/internal/adaptors/snippet"
	cb "insightify/internal/types/codebase"
)

// helper to create a temp repo file with given content
func writeTempFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestC4SnippetProviderCollectBFSWithTokens(t *testing.T) {
	root := t.TempDir()
	writeTempFile(t, root, "a.ts", "line1\nfoo();\nbar();\n")

	c4 := cb.C4Out{
		Files: []cb.IdentifierReport{
			{
				Path: "a.ts",
				Identifiers: []cb.IdentifierSignal{
					{
						Name:  "foo",
						Lines: [2]int{2, 2},
						Requires: []cb.IdentifierRequirement{
							{Path: "a.ts", Identifier: "bar"},
						},
					},
					{
						Name:  "bar",
						Lines: [2]int{3, 3},
					},
				},
			},
		},
	}

	prov := NewC4SnippetProvider(root, c4)
	q := snippet.Query{
		Seeds:     []snippet.Identifier{{Path: "a.ts", Name: "foo"}},
		MaxTokens: 50,
	}
	res, err := prov.Collect(context.Background(), q)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 snippets (foo + bar), got %d", len(res))
	}
	if !strings.Contains(res[0].Code, "foo") || !strings.Contains(res[1].Code, "bar") {
		t.Fatalf("unexpected code slices: %+v", res)
	}
}

func TestC4SnippetProviderRespectsMaxTokens(t *testing.T) {
	root := t.TempDir()
	writeTempFile(t, root, "a.ts", "line1\nfoo();\nbar();\n")

	c4 := cb.C4Out{
		Files: []cb.IdentifierReport{
			{
				Path: "a.ts",
				Identifiers: []cb.IdentifierSignal{
					{Name: "foo", Lines: [2]int{2, 2}, Requires: []cb.IdentifierRequirement{{Path: "a.ts", Identifier: "bar"}}},
					{Name: "bar", Lines: [2]int{3, 3}},
				},
			},
		},
	}

	prov := NewC4SnippetProvider(root, c4)
	q := snippet.Query{
		Seeds:       []snippet.Identifier{{Path: "a.ts", Name: "foo"}},
		MaxTokens:   7, // fits first snippet only with this simple counter
		CountTokens: func(s string) int { return len([]rune(s)) },
	}
	res, err := prov.Collect(context.Background(), q)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 snippet due to token cap, got %d", len(res))
	}
	if res[0].Signal.Name != "foo" {
		t.Fatalf("expected only foo, got %+v", res[0].Signal.Name)
	}
}

func TestC4SnippetProviderMissingLineSpanIsSkipped(t *testing.T) {
	root := t.TempDir()
	writeTempFile(t, root, "a.ts", "line1\nfoo();\n")

	c4 := cb.C4Out{
		Files: []cb.IdentifierReport{
			{
				Path:        "a.ts",
				Identifiers: []cb.IdentifierSignal{{Name: "foo", Lines: [2]int{0, 0}}},
			},
		},
	}

	prov := NewC4SnippetProvider(root, c4)
	q := snippet.Query{Seeds: []snippet.Identifier{{Path: "a.ts", Name: "foo"}}}
	res, err := prov.Collect(context.Background(), q)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("expected 0 snippets because line span missing, got %d", len(res))
	}
}
