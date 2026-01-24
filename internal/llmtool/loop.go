package llmtool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"insightify/internal/artifact"
	"insightify/internal/llmClient"
)

var (
	ErrMaxIterations  = errors.New("llmtool: max iterations reached")
	ErrUnknownAction  = errors.New("llmtool: unknown action")
	ErrToolNotFound   = errors.New("llmtool: tool not found")
	ErrToolNotAllowed = errors.New("llmtool: tool not allowed")
)

// ToolProvider abstracts tool registry calls.
type ToolProvider interface {
	Specs() []artifact.ToolSpec
	Call(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error)
}

// PromptBuilder builds the LLM prompt given tool specs and current tool state.
type PromptBuilder func(ctx context.Context, state *ToolState, tools []artifact.ToolSpec) (string, error)

// ToolLoop runs tool-call iterations until a final response is returned.
type ToolLoop struct {
	LLM      llmclient.LLMClient
	Tools    ToolProvider
	MaxIters int
	Allowed  []string
}

// ToolState captures tool results across iterations.
type ToolState struct {
	Input       any
	Iterations  int
	ToolResults []ToolResult
}

// ToolResult captures the output of a tool call.
type ToolResult struct {
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input,omitempty"`
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Run executes the tool loop and returns the final JSON result.
func (l *ToolLoop) Run(ctx context.Context, input any, build PromptBuilder) (json.RawMessage, *ToolState, error) {
	if l == nil || l.LLM == nil || l.Tools == nil {
		return nil, nil, fmt.Errorf("llmtool: missing LLM or tools")
	}
	if build == nil {
		return nil, nil, fmt.Errorf("llmtool: prompt builder is nil")
	}
	max := l.MaxIters
	if max <= 0 {
		max = 5
	}
	allowed := make(map[string]struct{}, len(l.Allowed))
	for _, a := range l.Allowed {
		a = strings.TrimSpace(a)
		if a != "" {
			allowed[a] = struct{}{}
		}
	}

	state := &ToolState{Input: input}
	tools := l.Tools.Specs()
	for i := 0; i < max; i++ {
		state.Iterations = i + 1
		prompt, err := build(ctx, state, tools)
		if err != nil {
			return nil, state, err
		}
		raw, err := l.LLM.GenerateJSON(ctx, prompt, input)
		if err != nil {
			return nil, state, err
		}
		action, err := ParseAction(raw)
		if err != nil {
			return nil, state, err
		}
		switch action.Action {
		case "final":
			return action.Final, state, nil
		case "tool":
			if action.ToolName == "" {
				return nil, state, fmt.Errorf("llmtool: tool_name required")
			}
			if len(allowed) > 0 {
				if _, ok := allowed[action.ToolName]; !ok {
					return nil, state, ErrToolNotAllowed
				}
			}
			out, err := l.Tools.Call(ctx, action.ToolName, action.ToolInput)
			tr := ToolResult{
				Name:   action.ToolName,
				Input:  action.ToolInput,
				Output: out,
			}
			if err != nil {
				tr.Error = err.Error()
			}
			state.ToolResults = append(state.ToolResults, tr)
			continue
		default:
			return nil, state, ErrUnknownAction
		}
	}
	return nil, state, ErrMaxIterations
}