package plan

import (
	"context"
	"testing"
)

func TestBootstrapRunGreeting(t *testing.T) {
	p := &BootstrapPipeline{}
	out, err := p.Run(context.Background(), BootstrapIn{
		UserInput: "",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if out.UINode.ID != "" {
		t.Fatalf("expected no ui node, got %q", out.UINode.ID)
	}
	if out.ClientView == nil || out.ClientView.GetLlmResponse() == "" {
		t.Fatalf("expected greeting llm_response")
	}
}
