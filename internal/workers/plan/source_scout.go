package plan

import (
	"context"
	"encoding/json"
	"strings"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/llmtool"
)

var sourceScoutPromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Recommend a suitable GitHub repository for the user's learning intent when possible.",
	Background:   "This stage extracts or proposes a concrete repository URL before the bootstrap planning response.",
	OutputFields: llmtool.MustFieldsFromStruct(artifact.PlanSourceScoutOut{}),
	Constraints: []string{
		"Extract a concrete GitHub repository URL from user_input when present, even if surrounded by extra text.",
		"If user_input already includes a GitHub repository URL, return that same URL in recommended_repo_url.",
		"If no concrete repository should be recommended, return an empty recommended_repo_url.",
		"Keep explanation concise and practical.",
	},
	Rules: []string{
		"Prefer concrete and popular repositories when recommendation is appropriate.",
		"Do not invent non-existent repository URLs.",
	},
	Assumptions:  []string{"When user intent is conceptual, recommendation may be omitted."},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent())

// SourceScout finds a candidate repository from user intent.
type SourceScout struct {
	LLM llmclient.LLMClient
}

func (p *SourceScout) Run(ctx context.Context, in artifact.PlanSourceScoutIn) (artifact.PlanSourceScoutOut, error) {
	var zero artifact.PlanSourceScoutOut
	input := strings.TrimSpace(in.UserInput)
	if input == "" || p == nil || p.LLM == nil {
		return zero, nil
	}

	llmCtx := llm.WithModelSelection(llm.WithWorker(ctx, "source_scout"), llm.ModelRoleWorker, llm.ModelLevelMiddle, "", "")
	payload := map[string]any{
		"user_input": input,
	}
	prompt, err := llmtool.StructuredPromptBuilder(sourceScoutPromptSpec)(llmCtx, &llmtool.ToolState{Input: payload}, nil)
	if err != nil {
		return zero, nil
	}
	raw, err := p.LLM.GenerateJSON(llmCtx, prompt, payload)
	if err != nil {
		return zero, nil
	}

	var out artifact.PlanSourceScoutOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, nil
	}
	out.RecommendedRepoURL = strings.TrimSpace(out.RecommendedRepoURL)
	out.Explanation = strings.TrimSpace(out.Explanation)
	return out, nil
}
