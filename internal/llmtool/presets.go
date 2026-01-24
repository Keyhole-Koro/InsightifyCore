package llmtool

// PromptPreset holds reusable constraints and rules for structured prompts.
type PromptPreset struct {
	Constraints []string
	Rules       []string
}

// ApplyPresets prepends preset constraints/rules to a structured prompt spec.
func ApplyPresets(spec StructuredPromptSpec, presets ...PromptPreset) StructuredPromptSpec {
	if len(presets) == 0 {
		return spec
	}
	var merged PromptPreset
	for _, p := range presets {
		merged.Constraints = append(merged.Constraints, p.Constraints...)
		merged.Rules = append(merged.Rules, p.Rules...)
	}
	spec.Constraints = append(merged.Constraints, spec.Constraints...)
	spec.Rules = append(merged.Rules, spec.Rules...)
	return spec
}

// PresetStrictJSON enforces strict JSON-only output.
func PresetStrictJSON() PromptPreset {
	return PromptPreset{
		Constraints: []string{
			"Return strict JSON only.",
			"Match the schema exactly; no extra fields.",
			"No markdown, comments, or trailing commas.",
		},
	}
}

// PresetNoInvent prevents fabricated paths/symbols.
func PresetNoInvent() PromptPreset {
	return PromptPreset{
		Constraints: []string{
			"Do not invent paths, filenames, symbols, or line ranges; use only provided inputs.",
		},
	}
}

// PresetCautious encourages explicit uncertainty.
func PresetCautious() PromptPreset {
	return PromptPreset{
		Rules: []string{
			"Avoid guessing; if unsure, make uncertainty explicit (notes, assumptions, or empty/null fields).",
		},
	}
}
