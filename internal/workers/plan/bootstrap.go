package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	pipelinev1 "insightify/gen/go/pipeline/v1"
	"insightify/internal/artifact"
	"insightify/internal/llm"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/llmtool"
	"insightify/internal/utils"
)

// BootstrapIn is the input for the bootstrap pipeline.
type BootstrapIn struct {
	UserInput   string                      `json:"user_input"`
	IsBootstrap bool                        `json:"is_bootstrap"`
	Scout       artifact.PlanSourceScoutOut `json:"scout"`
}

// BootstrapOut is the output of the bootstrap pipeline.
type BootstrapOut struct {
	Result     artifact.InitPurposeOut `json:"result"`
	ClientView *pipelinev1.ClientView  `json:"client_view,omitempty"`
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

type initPurposeNeedInputAdapter struct{}

func (initPurposeNeedInputAdapter) Extract(out artifact.InitPurposeOut) llmtool.NeedInputState {
	return llmtool.NeedInputState{
		NeedMoreInput:    out.NeedMoreInput,
		AssistantMessage: strings.TrimSpace(out.AssistantMessage),
		FollowupQuestion: strings.TrimSpace(out.FollowupQuestion),
	}
}

func (initPurposeNeedInputAdapter) Apply(out artifact.InitPurposeOut, state llmtool.NeedInputState) artifact.InitPurposeOut {
	out.NeedMoreInput = state.NeedMoreInput
	out.AssistantMessage = strings.TrimSpace(state.AssistantMessage)
	out.FollowupQuestion = strings.TrimSpace(state.FollowupQuestion)
	return out
}

var initPurposeNeedInputPolicy = llmtool.NeedInputPolicy{
	PreferFollowupAsMessage: true,
	RequireAssistantMessage: true,
	RequireFollowupQuestion: true,
	DefaultAssistantMessage: "Thanks. Could you share a bit more detail about your goal or the repository URL?",
	DefaultFollowupQuestion: "Could you share the repository URL or describe your learning goal more specifically?",
}

var initPurposePromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Collect user learning intent and optional repository target, then decide whether more input is needed.",
	Background:   "This stage returns the assistant response for the planning bootstrap conversation.",
	OutputFields: llmtool.MustFieldsFromStruct(artifact.InitPurposeOut{}),
	Constraints: []string{
		"assistant_message should be concise and practical English.",
		"assistant_message must never be empty.",
		"followup_question must never be empty.",
		"followup_question should be a short single question when need_more_input is true.",
		"repo_url must be a concrete GitHub URL if present; otherwise empty.",
		"purpose should be a short summarized learning goal.",
	},
	Rules: []string{
		"Use detected_repo_url and scout_explanation as hints, but prioritize user_input.",
		"If intent is still ambiguous, set need_more_input=true.",
		"If assistant_message would otherwise be empty, use: Thanks. Could you share a bit more detail about your goal or the repository URL?",
		"If followup_question would otherwise be empty, use: Could you share the repository URL or describe your learning goal more specifically?",
	},
	Assumptions:  []string{"If both repo_url and purpose are empty, more input is required."},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent())

// Run executes the bootstrap pipeline.
func (p *BootstrapPipeline) Run(ctx context.Context, in BootstrapIn) (BootstrapOut, error) {
	if p == nil {
		return BootstrapOut{}, fmt.Errorf("bootstrap: pipeline is nil")
	}

	var out BootstrapOut

	result, err := p.runBootstrap(ctx, in)
	if err != nil {
		return out, err
	}

	out.Result = result
	out.ClientView = buildClientView(result)
	return out, nil
}

func (p *BootstrapPipeline) runBootstrap(ctx context.Context, in BootstrapIn) (artifact.InitPurposeOut, error) {
	// Initial greeting when no user input yet
	if in.IsBootstrap && strings.TrimSpace(in.UserInput) == "" {
		msg := "Would you like to explore how computers work, or dive into real OSS code to deepen your understanding? Share a topic you're curious about or paste a GitHub repository URL."
		p.emitChunk(msg)
		return artifact.InitPurposeOut{
			AssistantMessage: msg,
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
	extractedRepo := strings.TrimSpace(in.Scout.RecommendedRepoURL)
	scoutExplanation := strings.TrimSpace(in.Scout.Explanation)

	// Run the main bootstrap LLM call
	result, err := p.runBootstrapLLM(ctx, input, extractedRepo, scoutExplanation)
	if err != nil {
		return artifact.InitPurposeOut{}, err
	}
	return result, nil
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
	out = llmtool.NormalizeNeedInput(llmCtx, out, initPurposeNeedInputAdapter{}, initPurposeNeedInputPolicy, nil)
	return out, nil
}

func buildClientView(result artifact.InitPurposeOut) *pipelinev1.ClientView {
	view := &pipelinev1.ClientView{
		Phase:   "bootstrap",
		Content: &pipelinev1.ClientView_LlmResponse{LlmResponse: strings.TrimSpace(result.AssistantMessage)},
	}
	if view.GetGraph() != nil {
		// Keep compatibility with graph-based workers if content kind changes in future.
		utils.AssignGraphNodeUIDs(view)
	}
	return view
}
