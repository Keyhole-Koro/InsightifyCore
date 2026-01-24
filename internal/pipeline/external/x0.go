package external

import (
	"context"
	"encoding/json"
	"fmt"

	"insightify/internal/artifact"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/llmtool"
)

var x0PromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Synthesize an external/infrastructure map using repository signals.",
	Background:   "Stage X0 synthesizes an external/infrastructure map using signals from roots, architecture, config samples, and identifier summaries.",
	OutputFields: llmtool.MustFieldsFromStruct(artifact.X0Out{}),
	Constraints: []string{
		"Use only repository-relative paths exactly as provided in inputs. If a sample path is absolute, emit a repo-relative equivalent (or the given path if unknown). Never invent files.",
		"Cite evidence using {path, lines}. When line numbers cannot be determined (e.g., from summaries), set 'lines': null.",
		"Focus external systems/APIs/cloud services/infrastructure tooling. Mention Terraform/SAM/CloudFormation/etc when present.",
		"Summaries must stay concise (1-2 sentences). Avoid repeating identical info across fields.",
		"confidence_threshold indicates when a hypothesis needs investigation. Only emit 'suggested' lookups when confidence < threshold.",
		"Each suggestion should instruct whether to open a full file ('kind': 'file') or a specific identifier snippet ('kind': 'identifier', include identifier name).",
		"When unsure, explain why in 'impact' or 'notes' rather than guessing.",
	},
	Rules: []string{
		"Identify external systems, infra components, build/deploy tools, and runtime configs.",
		"Assess confidence for each identified item.",
		"Identify evidence gaps where confidence is low.",
	},
	Assumptions:  []string{"Missing info implies lower confidence."},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent())

// X0 orchestrates the external-context reasoning step.
type X0 struct {
	LLM llmclient.LLMClient
}

// Run executes Stage X0 with defensive guards around the LLM call.
func (p *X0) Run(ctx context.Context, in artifact.X0In) (artifact.X0Out, error) {
	if p == nil || p.LLM == nil {
		return artifact.X0Out{}, fmt.Errorf("x0: llm client is nil")
	}
	if in.ConfidenceThreshold <= 0 {
		in.ConfidenceThreshold = 0.65
	}
	const (
		maxSamples     = 16
		maxIdentifiers = 40
	)
	if len(in.ConfigSamples) > maxSamples {
		in.ConfigSamples = cloneOpenedFiles(in.ConfigSamples[:maxSamples])
	}
	if len(in.IdentifierSummaries) > maxIdentifiers {
		in.IdentifierSummaries = cloneIdentifierSummaries(in.IdentifierSummaries[:maxIdentifiers])
	}
	payload := map[string]any{
		"repo":                 in.Repo,
		"roots":                in.Roots,
		"architecture":         in.Architecture,
		"config_samples":       in.ConfigSamples,
		"identifier_summaries": in.IdentifierSummaries,
		"confidence_threshold": in.ConfidenceThreshold,
	}

	prompt, err := llmtool.StructuredPromptBuilder(x0PromptSpec)(ctx, &llmtool.ToolState{Input: payload}, nil)
	if err != nil {
		return artifact.X0Out{}, err
	}

	raw, err := p.LLM.GenerateJSON(ctx, prompt, payload)
	if err != nil {
		return artifact.X0Out{}, err
	}
	var out artifact.X0Out
	if err := json.Unmarshal(raw, &out); err != nil {
		return artifact.X0Out{}, fmt.Errorf("X0 JSON invalid: %w\nraw: %s", err, string(raw))
	}
	return out, nil
}

func cloneOpenedFiles(in []artifact.OpenedFile) []artifact.OpenedFile {
	out := make([]artifact.OpenedFile, len(in))
	copy(out, in)
	return out
}

func cloneIdentifierSummaries(in []artifact.IdentifierSummary) []artifact.IdentifierSummary {
	out := make([]artifact.IdentifierSummary, len(in))
	copy(out, in)
	return out
}
