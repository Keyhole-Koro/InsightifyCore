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
	// Heuristic: if no explicit action/tool/final fields are found,
	// assume the entire raw JSON is the final output.
	if env.Action == "" && env.ToolName == "" && len(env.Final) == 0 {
		// Verify it's not just an empty JSON object "{}" if that matters,
		// but usually treating it as final is the safest fallback for "direct output" models.
		env.Action = "final"
		env.Final = raw
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
