package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	workerv1 "insightify/gen/go/worker/v1"
	"insightify/internal/artifact"
	"insightify/internal/llm"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/llmtool"
	"insightify/internal/ui"
)

// BootstrapIn is the input for the bootstrap pipeline.
type BootstrapIn struct {
	UserInput string `json:"user_input"`
}

// BootstrapOut is the output of the bootstrap pipeline.
type BootstrapOut struct {
	Result           artifact.InitPurposeOut   `json:"result"`
	BootstrapContext artifact.BootstrapContext `json:"bootstrap_context"`
	ClientView       *workerv1.ClientView      `json:"client_view,omitempty"`
	UINode           ui.Node                   `json:"ui_node,omitempty"`
}

// NeedMoreInput returns true if more user input is required.
func (o BootstrapOut) NeedMoreInput() bool {
	return o.Result.NeedMoreInput
}

// ChunkEmitter emits LLM streaming chunks. Implemented by runner.RunEventEmitter.
type ChunkEmitter interface {
	EmitLLMChunk(chunk string)
}

// BootstrapPipeline collects user intent and repository information.
type BootstrapPipeline struct {
	LLM     llmclient.LLMClient
	Emitter ChunkEmitter
}

var initPurposePromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Collect user learning intent and optional repository target, then decide whether more input is needed.",
	Background:   "This stage returns the assistant response for the planning bootstrap conversation.",
	OutputFields: llmtool.MustFieldsFromStruct(artifact.InitPurposeOut{}),
	Constraints: []string{
		"followup_question must never be empty.",
		"followup_question should be a short single question when need_more_input is true.",
		"repo_url must be a concrete GitHub URL if present; otherwise empty.",
		"purpose should be a short summarized learning goal.",
	},
	Rules: []string{
		"Use detected_repo_url and scout_explanation as hints, but prioritize user_input.",
		"If intent is still ambiguous, set need_more_input=true.",
	},
	Assumptions:  []string{"If both repo_url and purpose are empty, more input is required."},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent())

const bootstrapGreetingMessage = "Would you like to explore how computers work, or dive into real OSS code to deepen your understanding? Share a topic you're curious about or paste a GitHub repository URL."

type bootstrapScoutResult struct {
	RecommendedRepoURL string `json:"recommended_repo_url"`
	Explanation        string `json:"explanation"`
}

var bootstrapScoutPromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Recommend a suitable GitHub repository for the user's learning intent when possible.",
	Background:   "This stage extracts or proposes a concrete repository URL before the bootstrap planning response.",
	OutputFields: llmtool.MustFieldsFromStruct(bootstrapScoutResult{}),
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

// Run executes the bootstrap pipeline.
func (p *BootstrapPipeline) Run(ctx context.Context, in BootstrapIn) (BootstrapOut, error) {
	if p == nil {
		return BootstrapOut{}, fmt.Errorf("bootstrap: pipeline is nil")
	}

	out := BootstrapOut{}

	result, err := p.runBootstrap(ctx, in)
	if err != nil {
		return out, err
	}

	out.Result = result
	out.BootstrapContext = artifact.BootstrapContext{
		Purpose:   result.Purpose,
		RepoURL:   result.RepoURL,
		UserInput: strings.TrimSpace(in.UserInput),
	}.Normalize()
	out.ClientView = buildClientView(result)
	return out, nil
}

func (p *BootstrapPipeline) runBootstrap(ctx context.Context, in BootstrapIn) (artifact.InitPurposeOut, error) {
	// Initial greeting when no user input yet.
	if strings.TrimSpace(in.UserInput) == "" {
		p.emitChunk(bootstrapGreetingMessage)
		return artifact.InitPurposeOut{
			FollowupQuestion: bootstrapGreetingMessage,
			NeedMoreInput:    true,
		}, nil
	}

	input := strings.TrimSpace(in.UserInput)
	if input == "" {
		return artifact.InitPurposeOut{}, fmt.Errorf("bootstrap: input is required")
	}
	if p.LLM == nil {
		return artifact.InitPurposeOut{}, fmt.Errorf("bootstrap: llm client is nil")
	}

	ctx = llm.WithWorker(ctx, "bootstrap")
	scout := p.resolveScout(ctx, input)
	extractedRepo := strings.TrimSpace(scout.RecommendedRepoURL)
	scoutExplanation := strings.TrimSpace(scout.Explanation)

	// Run the main bootstrap LLM call
	result, err := p.runBootstrapLLM(ctx, input, extractedRepo, scoutExplanation)
	if err != nil {
		return artifact.InitPurposeOut{}, err
	}
	return result, nil
}

func (p *BootstrapPipeline) resolveScout(ctx context.Context, input string) bootstrapScoutResult {
	if strings.TrimSpace(input) == "" || p == nil || p.LLM == nil {
		return bootstrapScoutResult{}
	}
	scout, err := p.runScoutLLM(ctx, input)
	if err != nil {
		return bootstrapScoutResult{}
	}
	return scout
}

func (p *BootstrapPipeline) emitChunk(chunk string) {
	if p.Emitter != nil && strings.TrimSpace(chunk) != "" {
		p.Emitter.EmitLLMChunk(chunk)
	}
}

// --- Internal helpers ---

func (p *BootstrapPipeline) runBootstrapLLM(ctx context.Context, userInput, detectedRepoURL, scoutExplanation string) (artifact.InitPurposeOut, error) {
	if p.LLM == nil {
		return artifact.InitPurposeOut{}, fmt.Errorf("bootstrap: llm client is nil")
	}
	payload := map[string]any{
		"user_input":        userInput,
		"detected_repo_url": detectedRepoURL,
		"scout_explanation": scoutExplanation,
	}
	llmCtx := llm.WithModelSelection(ctx, llm.ModelRoleWorker, llm.ModelLevelLow, "", "")
	prompt, err := llmtool.StructuredPromptBuilder(initPurposePromptSpec)(llmCtx, &llmtool.ToolState{Input: payload}, nil)
	if err != nil {
		return artifact.InitPurposeOut{}, err
	}
	raw, err := p.LLM.GenerateJSONStream(llmCtx, prompt, payload, p.emitChunk)
	if err != nil {
		return artifact.InitPurposeOut{}, err
	}
	var out artifact.InitPurposeOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return artifact.InitPurposeOut{}, fmt.Errorf("Bootstrap JSON invalid: %w\nraw: %s", err, string(raw))
	}
	return out, nil
}

func (p *BootstrapPipeline) runScoutLLM(ctx context.Context, userInput string) (bootstrapScoutResult, error) {
	if p.LLM == nil {
		return bootstrapScoutResult{}, fmt.Errorf("bootstrap: llm client is nil")
	}
	payload := map[string]any{
		"user_input": strings.TrimSpace(userInput),
	}
	llmCtx := llm.WithModelSelection(llm.WithWorker(ctx, "source_scout"), llm.ModelRoleWorker, llm.ModelLevelMiddle, "", "")
	prompt, err := llmtool.StructuredPromptBuilder(bootstrapScoutPromptSpec)(llmCtx, &llmtool.ToolState{Input: payload}, nil)
	if err != nil {
		return bootstrapScoutResult{}, err
	}
	raw, err := p.LLM.GenerateJSON(llmCtx, prompt, payload)
	if err != nil {
		return bootstrapScoutResult{}, err
	}
	var out bootstrapScoutResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return bootstrapScoutResult{}, fmt.Errorf("bootstrap scout json invalid: %w", err)
	}
	out.RecommendedRepoURL = strings.TrimSpace(out.RecommendedRepoURL)
	out.Explanation = strings.TrimSpace(out.Explanation)
	return out, nil
}

func buildClientView(result artifact.InitPurposeOut) *workerv1.ClientView {
	return &workerv1.ClientView{
		Phase:   "bootstrap",
		Content: &workerv1.ClientView_LlmResponse{LlmResponse: strings.TrimSpace(result.FollowupQuestion)},
	}
}
