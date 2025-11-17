package adaptors

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"insightify/internal/safeio"
	cb "insightify/internal/types/codebase"
)

func TestC4SnippetProviderCollect(t *testing.T) {
	root := t.TempDir()
	fs, err := safeio.NewSafeFS(root)
	if err != nil {
		t.Fatalf("safe fs: %v", err)
	}
	safeio.SetDefault(fs)
	rel := "src/main.ts"
	abs := filepath.Join(root, filepath.FromSlash(rel))
	content := "line1\nfoo()\nbar()\n"
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	c4 := cb.C4Out{
		Files: []cb.IdentifierReport{
			{
				Path: rel,
				Identifiers: []cb.IdentifierSignal{
					{
						Name:  "foo",
						Lines: [2]int{2, 2},
						Requires: []cb.IdentifierRequirement{
							{Path: rel, Identifier: "bar"},
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

	provider := NewC4SnippetProvider(root, c4)
	snips, errs := provider.Collect([]IdentifierSelector{{Path: rel, Identifier: "foo"}})

	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(snips) != 2 {
		t.Fatalf("expected 2 snippets, got %d", len(snips))
	}
	if snips[0].FilePath != rel || snips[1].FilePath != rel {
		t.Fatalf("unexpected file paths: %v", snips)
	}
	if got := snips[0].Code; !strings.Contains(got, "foo()") {
		t.Fatalf("foo snippet missing code: %q", got)
	}
	if got := snips[1].Code; !strings.Contains(got, "bar()") {
		t.Fatalf("bar snippet missing code: %q", got)
	}
}

func TestC4SnippetProviderMissingSymbol(t *testing.T) {
	root := t.TempDir()
	fs, err := safeio.NewSafeFS(root)
	if err != nil {
		t.Fatalf("safe fs: %v", err)
	}
	safeio.SetDefault(fs)
	provider := NewC4SnippetProvider(root, cb.C4Out{})
	_, errs := provider.Collect([]IdentifierSelector{{Path: "missing.ts", Identifier: "noop"}})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	for sel, err := range errs {
		if !strings.Contains(err.Error(), sel.Path) || !strings.Contains(err.Error(), sel.Identifier) {
			t.Fatalf("error should mention selector, got: %v", err)
		}
	}
}
