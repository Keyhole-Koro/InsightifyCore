package external

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"insightify/internal/artifact"
	llmclient "insightify/internal/llmclient"
	"insightify/internal/llmtool"
)

type infraRefinePromptOut struct {
	Delta      artifact.InfraRefineDelta `json:"delta" prompt_desc:"Changes vs previous hypothesis (added, removed, modified)."`
	NeedsInput []string         `json:"needs_input" prompt_desc:"Questions or requests for more input."`
	StopWhen   []string         `json:"stop_when" prompt_desc:"Convergence criteria."`
	Notes      []string         `json:"notes" prompt_desc:"Short notes or caveats."`
}

var infraRefinePromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Refine the external architecture hypothesis based on new evidence.",
	Background:   "Stage InfraRefine interprets new evidence to refine or correct the external architecture hypothesis (purpose, components, integrations, configs, infra).",
	OutputFields: llmtool.MustFieldsFromStruct(infraRefinePromptOut{}),
	Constraints: []string{
		"Report only the *deltas* vs. previous_result.",
		"Use field paths referencing previous_result (e.g., 'external_overview.external_systems[1].interaction').",
		"'before'/'after' snapshots should stay concise (strings or short JSON). Cite evidence using {path, lines:[start,end]|null}.",
		"Keep delta.added/removed for high-level statements (e.g., 'Added AWS EventBridge trigger').",
		"If everything is resolved, return empty arrays for needs_input and stop_when with notes describing confidence.",
	},
	Rules: []string{
		"Interpret the new evidence to refine or correct the external architecture hypothesis.",
		"Flag unresolved questions under needs_input with concrete follow-up actions (e.g., 'file:template.yaml reason=check IAM policies').",
	},
	Assumptions:  []string{"Assume previous hypothesis is the baseline."},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent())

// InfraRefine consumes additional evidence to close open questions from X0.
type InfraRefine struct {
	LLM llmclient.LLMClient
}

func (p *InfraRefine) Run(ctx context.Context, in artifact.InfraRefineIn) (artifact.InfraRefineOut, error) {
	if p == nil || p.LLM == nil {
		return artifact.InfraRefineOut{}, fmt.Errorf("infraRefine: llm client is nil")
	}
	const maxEvidence = 24
	if len(in.Files) > maxEvidence {
		in.Files = cloneOpenedFiles(in.Files[:maxEvidence])
	}
	payload := map[string]any{
		"repo":            in.Repo,
		"previous_result": in.Previous,
		"file_evidence":   in.Files,
		"notes":           in.Notes,
	}

	prompt, err := llmtool.StructuredPromptBuilder(infraRefinePromptSpec)(ctx, &llmtool.ToolState{Input: payload}, nil)
	if err != nil {
		return artifact.InfraRefineOut{}, err
	}

	raw, err := p.LLM.GenerateJSON(ctx, prompt, payload)
	if err != nil {
		return artifact.InfraRefineOut{}, err
	}
	var out artifact.InfraRefineOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return artifact.InfraRefineOut{}, fmt.Errorf("InfraRefine JSON invalid: %w\nraw: %s", err, string(raw))
	}
	out.ExternalOverview = applyExternalDelta(in.Previous, out.Delta)
	return out, nil
}

type pathToken struct {
	Key   string
	Index *int
}

func applyExternalDelta(prev artifact.InfraContextOut, delta artifact.InfraRefineDelta) artifact.ExternalOverview {
	root := structToJSONMap(prev)
	for _, mod := range delta.Modified {
		if mod.Field == "" {
			continue
		}
		if err := setJSONValue(root, mod.Field, mod.After); err != nil {
			continue
		}
	}
	return extractExternalOverview(root)
}

func structToJSONMap(v any) map[string]any {
	var out map[string]any
	data, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	_ = json.Unmarshal(data, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func extractExternalOverview(root map[string]any) artifact.ExternalOverview {
	var eo artifact.ExternalOverview
	raw, ok := root["external_overview"]
	if !ok {
		return eo
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return eo
	}
	_ = json.Unmarshal(data, &eo)
	return eo
}

func setJSONValue(root map[string]any, field string, value any) error {
	if root == nil {
		return fmt.Errorf("nil root")
	}
	tokens := parsePathTokens(field)
	if len(tokens) == 0 {
		return fmt.Errorf("empty field")
	}
	var current any = root
	for idx, tok := range tokens {
		last := idx == len(tokens)-1
		switch node := current.(type) {
		case map[string]any:
			next, exists := node[tok.Key]
			if tok.Index != nil {
				arr, ok := next.([]any)
				if !exists || !ok {
					arr = make([]any, 0, *tok.Index+1)
				}
			targetIdx := *tok.Index
			for len(arr) <= targetIdx {
				arr = append(arr, nil)
			}
			if last {
				arr[targetIdx] = value
				node[tok.Key] = arr
				return nil
			}
			if arr[targetIdx] == nil {
				arr[targetIdx] = map[string]any{}
			}
			current = arr[targetIdx]
			node[tok.Key] = arr
			continue
		}
		if !exists {
			if last {
				node[tok.Key] = value
				return nil
			}
			next = map[string]any{}
			node[tok.Key] = next
		}
		if last {
			node[tok.Key] = value
			return nil
		}
		if child, ok := next.(map[string]any); ok {
			current = child
		} else {
			child = map[string]any{}
			node[tok.Key] = child
			current = child
		}
	case []any:
		if tok.Index == nil {
			return fmt.Errorf("array segment missing index for %s", tok.Key)
		}
		targetIdx := *tok.Index
		arr := node
		for len(arr) <= targetIdx {
			arr = append(arr, nil)
		}
		if last {
			arr[targetIdx] = value
			current = arr
			continue
		}
		if arr[targetIdx] == nil {
			arr[targetIdx] = map[string]any{}
		}
		current = arr[targetIdx]
	default:
			return fmt.Errorf("invalid type in path")
		}
	}
	return nil
}

func parsePathTokens(field string) []pathToken {
	var tokens []pathToken
	parts := strings.Split(field, ".")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		for len(part) > 0 {
			idx := strings.Index(part, "[")
			if idx == -1 {
				tokens = append(tokens, pathToken{Key: part})
				break
			}
			key := part[:idx]
			key = strings.TrimSpace(key)
			end := strings.Index(part[idx:], "]")
			if end == -1 {
				tokens = append(tokens, pathToken{Key: part})
				break
			}
			endIdx := idx + end
			numStr := strings.TrimSpace(part[idx+1 : endIdx])
			num := 0
			fmt.Sscanf(numStr, "%d", &num)
			idxCopy := num
			tokens = append(tokens, pathToken{Key: key, Index: &idxCopy})
			part = part[endIdx+1:]
			if len(part) == 0 {
				break
			}
			if part[0] == '.' {
				part = part[1:]
			}
		}
	}
	return tokens
}
