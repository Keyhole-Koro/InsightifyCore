package ui

import "testing"

func TestBuildLLMChatNode(t *testing.T) {
	node, ok := BuildLLMChatNode("testllmChar-1", "", "hello", 7, true, false, "")
	if !ok {
		t.Fatalf("expected node")
	}
	if node.ID != "testllmChar-1" {
		t.Fatalf("unexpected id: %q", node.ID)
	}
	if node.Meta.Title != "testllmChar" {
		t.Fatalf("unexpected title: %q", node.Meta.Title)
	}
	if node.LLMChat == nil || !node.LLMChat.IsResponding {
		t.Fatalf("expected responding")
	}
	if len(node.LLMChat.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(node.LLMChat.Messages))
	}
	if node.LLMChat.Messages[0].Role != RoleAssistant {
		t.Fatalf("unexpected role: %q", node.LLMChat.Messages[0].Role)
	}
}

func TestBuildOtherUINodes(t *testing.T) {
	md, ok := BuildMarkdownNode("md-1", "Doc", "# hello")
	if !ok || md.Markdown == nil {
		t.Fatalf("expected markdown node")
	}
	img, ok := BuildImageNode("img-1", "Image", "https://example.com/a.png", "alt")
	if !ok || img.Image == nil {
		t.Fatalf("expected image node")
	}
	tbl, ok := BuildTableNode("tbl-1", "Table", []string{"a"}, [][]string{{"1"}})
	if !ok || tbl.Table == nil || len(tbl.Table.Rows) != 1 {
		t.Fatalf("expected table node")
	}
}

func TestBuildLLMChatNodeEmptyRunID(t *testing.T) {
	if _, ok := BuildLLMChatNode("   ", "x", "hello", 1, false, false, ""); ok {
		t.Fatalf("expected no node for empty run id")
	}
}
