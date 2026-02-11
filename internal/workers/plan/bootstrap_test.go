package plan

import (
	"context"
	"testing"
)

func TestBootstrapRunCreatesInitialAndFinalNode(t *testing.T) {
	p := &BootstrapPipeline{}
	out, err := p.Run(context.Background(), BootstrapIn{
		UserInput: "",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if out.UINode.ID != "init-purpose-node" {
		t.Fatalf("unexpected node id: %q", out.UINode.ID)
	}
	if out.UINode.LLMChat == nil {
		t.Fatalf("expected llm chat state")
	}
	if out.UINode.Meta.Title != "bootstrap" {
		t.Fatalf("expected bootstrap node title, got %q", out.UINode.Meta.Title)
	}
	if len(out.UINode.LLMChat.Messages) == 0 {
		t.Fatalf("expected assistant greeting message in final node")
	}
	if !out.Result.NeedMoreInput {
		t.Fatalf("expected need_more_input=true for bootstrap greeting")
	}
}
