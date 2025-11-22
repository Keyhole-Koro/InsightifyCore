package external

import (
	"context"
	"encoding/json"
	"fmt"

	llmclient "insightify/internal/llmClient"
	t "insightify/internal/types"
	ex "insightify/internal/types/external"
)

const promptX0 = `You are **Stage X0 (external)** of a repository analysis pipeline.

Goal: synthesize an external/infrastructure map using signals from:
- roots: directories/files grouped by main/build/config/runtime classes (from M0)
- architecture: current high-level hypothesis (from M1)
- config_samples: raw text for infra/config/build files (Terraform, SAM, GitHub Actions, env templates, etc.)
- identifier_summaries: summaries of identifiers near external dependencies (from C4)
- confidence_threshold: numeric threshold for what counts as "low confidence"

You must return STRICT JSON (no comments, no extra fields) with schema:
{
  "external_overview": {
    "purpose": "string",
    "architecture_summary": "string",
    "external_systems": [
      {
        "name": "string",
        "kind": "string",
        "interaction": "string",
        "evidence": [{"path":"string","lines":[1,2]|null}],
        "confidence": 0.0
      }
    ],
    "infra_components": [
      {
        "name": "string",
        "type": "string",
        "paths": ["string"],
        "summary": "string",
        "evidence": [{"path":"string","lines":[1,2]|null}],
        "confidence": 0.0
      }
    ],
    "build_and_deploy": [
      {
        "tool": "string",
        "usage": "string",
        "evidence": [{"path":"string","lines":[1,2]|null}],
        "confidence": 0.0
      }
    ],
    "runtime_configs": [
      {
        "path": "string",
        "description": "string",
        "evidence": [{"path":"string","lines":[1,2]|null}],
        "confidence": 0.0
      }
    ],
    "confidence": 0.0
  },
  "evidence_gaps": [
    {
      "topic": "string",
      "question": "string",
      "current_guess": "string",
      "impact": "string",
      "confidence": 0.0,
      "suggested": [
        {
          "kind": "file|identifier|config|doc",
          "path": "string",
          "identifier": "string",
          "reason": "string",
          "notes": "string"
        }
      ]
    }
  ],
  "notes": ["string"]
}

Rules:
- Use only repository-relative paths exactly as provided in inputs. If a sample path is absolute, emit a repo-relative equivalent (or the given path if unknown). Never invent files.
- Cite evidence using {path, lines}. When line numbers cannot be determined (e.g., from summaries), set "lines": null.
- Focus external systems/APIs/cloud services/infrastructure tooling. Mention Terraform/SAM/CloudFormation/etc when present.
- Summaries must stay concise (1-2 sentences). Avoid repeating identical info across fields.
- confidence_threshold indicates when a hypothesis needs investigation. Only emit "suggested" lookups when confidence < threshold. Each suggestion should instruct whether to open a full file ("kind": "file") or a specific identifier snippet ("kind": "identifier", include identifier name).
- When unsure, explain why in "impact" or "notes" rather than guessing.
- JSON only. No markdown, bullets, or trailing commas.
`

// X0 orchestrates the external-context reasoning step.
type X0 struct {
	LLM llmclient.LLMClient
}

// Run executes Stage X0 with defensive guards around the LLM call.
func (p *X0) Run(ctx context.Context, in ex.X0In) (ex.X0Out, error) {
	if p == nil || p.LLM == nil {
		return ex.X0Out{}, fmt.Errorf("x0: llm client is nil")
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
	raw, err := p.LLM.GenerateJSON(ctx, promptX0, payload)
	if err != nil {
		return ex.X0Out{}, err
	}
	var out ex.X0Out
	if err := json.Unmarshal(raw, &out); err != nil {
		return ex.X0Out{}, fmt.Errorf("X0 JSON invalid: %w\nraw: %s", err, string(raw))
	}
	return out, nil
}

func cloneOpenedFiles(in []t.OpenedFile) []t.OpenedFile {
	out := make([]t.OpenedFile, len(in))
	copy(out, in)
	return out
}

func cloneIdentifierSummaries(in []ex.IdentifierSummary) []ex.IdentifierSummary {
	out := make([]ex.IdentifierSummary, len(in))
	copy(out, in)
	return out
}
