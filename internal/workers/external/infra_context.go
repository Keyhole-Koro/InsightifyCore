package external

import (
	"context"
	"encoding/json"
	"fmt"

	"insightify/internal/artifact"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/llmtool"
	"insightify/internal/common/safeio"
)

var infraContextPromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Synthesize an external/infrastructure map using repository signals.",
	Background:   "Stage InfraContext synthesizes an external/infrastructure map using signals from roots, architecture, config samples, and identifier summaries.",
	OutputFields: llmtool.MustFieldsFromStruct(artifact.InfraContextOut{}),
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

// InfraContext orchestrates the external-context reasoning step.
type InfraContext struct {
	LLM    llmclient.LLMClient
	RepoFS *safeio.SafeFS
}

// Run executes Stage InfraContext with defensive guards around the LLM call.
func (p *InfraContext) Run(ctx context.Context, in artifact.InfraContextIn) (artifact.InfraContextOut, error) {
	if p == nil || p.LLM == nil {
		return artifact.InfraContextOut{}, fmt.Errorf("infraContext: llm client is nil")
	}
	if in.ConfidenceThreshold <= 0 {
		in.ConfidenceThreshold = 0.65
	}
	const (
		maxSamples     = 16
		maxSampleBytes = 16000
		maxIdentifiers = 40
	)
	if len(in.ConfigSamples) == 0 && p.RepoFS != nil {
		in.ConfigSamples = CollectInfraSamples(p.RepoFS, in.Repo, in.Roots, maxSamples, maxSampleBytes)
	}
	if len(in.IdentifierSummaries) == 0 {
		in.IdentifierSummaries = SelectIdentifierSummaries(in.IdentifierReports, in.Repo, in.Roots, maxIdentifiers)
	}
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

	prompt, err := llmtool.StructuredPromptBuilder(infraContextPromptSpec)(ctx, &llmtool.ToolState{Input: payload}, nil)
	if err != nil {
		return artifact.InfraContextOut{}, err
	}

	raw, err := p.LLM.GenerateJSON(ctx, prompt, payload)
	if err != nil {
		return artifact.InfraContextOut{}, err
	}
	var out artifact.InfraContextOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return artifact.InfraContextOut{}, fmt.Errorf("InfraContext JSON invalid: %w\nraw: %s", err, string(raw))
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
