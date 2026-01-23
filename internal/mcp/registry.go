package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ToolSpec documents a tool's contract (name + schemas).
type ToolSpec struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}

// Tool is a minimal in-process MCP-style tool.
type Tool interface {
	Spec() ToolSpec
	Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

// Registry holds tool registrations and dispatches calls.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates an empty registry and registers any provided tools.
func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: map[string]Tool{}}
	for _, t := range tools {
		r.Register(t)
	}
	return r
}

// Register adds or replaces a tool by name.
func (r *Registry) Register(t Tool) {
	if r == nil || t == nil {
		return
	}
	spec := t.Spec()
	if spec.Name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tools == nil {
		r.tools = map[string]Tool{}
	}
	r.tools[spec.Name] = t
}

// Call invokes a registered tool.
func (r *Registry) Call(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
	if r == nil {
		return nil, fmt.Errorf("mcp: registry is nil")
	}
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("mcp: unknown tool %q", name)
	}
	return t.Call(ctx, input)
}

// Specs returns the current tool specs.
func (r *Registry) Specs() []ToolSpec {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ToolSpec, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Spec())
	}
	return out
}
