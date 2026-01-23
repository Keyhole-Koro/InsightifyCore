package llmtool

import (
	"encoding/json"
	"fmt"
)

// ActionEnvelope describes the tool-loop action response from the LLM.
type ActionEnvelope struct {
	Action    string          `json:"action,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
	Final     json.RawMessage `json:"final,omitempty"`
}

// ParseAction parses the LLM response into an action envelope.
func ParseAction(raw json.RawMessage) (ActionEnvelope, error) {
	var env ActionEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return ActionEnvelope{}, err
	}
	if env.Action == "" {
		switch {
		case len(env.Final) > 0:
			env.Action = "final"
		case env.ToolName != "" || len(env.ToolInput) > 0:
			env.Action = "tool"
		}
	}
	switch env.Action {
	case "final", "tool":
		return env, nil
	default:
		return ActionEnvelope{}, fmt.Errorf("llmtool: invalid action %q", env.Action)
	}
}
