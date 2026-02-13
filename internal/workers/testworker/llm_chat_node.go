package testpipe

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	pipelinev1 "insightify/gen/go/pipeline/v1"
	"insightify/internal/artifact"
	"insightify/internal/llm"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/llmtool"
	"insightify/internal/ui"
	"insightify/internal/workers/plan"
)

const (
	testChatNodeID    = "test-llm-chat-node"
	testChatWorkerKey = "testllmChatNode"
	testChatModelName = "Low"
)

type chatNeedInputAdapter struct{}

func (chatNeedInputAdapter) Extract(out artifact.InitPurposeOut) llmtool.NeedInputState {
	return llmtool.NeedInputState{
		NeedMoreInput:    out.NeedMoreInput,
		AssistantMessage: strings.TrimSpace(out.AssistantMessage),
		FollowupQuestion: strings.TrimSpace(out.FollowupQuestion),
	}
}

func (chatNeedInputAdapter) Apply(out artifact.InitPurposeOut, state llmtool.NeedInputState) artifact.InitPurposeOut {
	out.NeedMoreInput = state.NeedMoreInput
	out.AssistantMessage = strings.TrimSpace(state.AssistantMessage)
	out.FollowupQuestion = strings.TrimSpace(state.FollowupQuestion)
	return out
}

var chatNeedInputPolicy = llmtool.NeedInputPolicy{
	PreferFollowupAsMessage: true,
}

var chatPromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Carry out a casual daily conversation with the user while deciding whether to continue or end.",
	Background:   "This worker powers an interactive chat node and returns one assistant turn per call.",
	OutputFields: llmtool.MustFieldsFromStruct(artifact.InitPurposeOut{}),
	Constraints: []string{
		"assistant_message should be concise and natural.",
		"If the user clearly ends the chat (e.g., bye), set need_more_input=false and followup_question=''.",
	},
	Rules: []string{
		"Prioritize the latest user_input.",
		"When continuing conversation, set need_more_input=true and ask one clear follow-up question.",
	},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent())

// ChunkEmitter emits LLM streaming chunks.
type ChunkEmitter interface {
	EmitLLMChunk(chunk string)
}

// LLMChatNodePipeline is an interactive chat worker backed by LLM.
type LLMChatNodePipeline struct {
	LLM     llmclient.LLMClient
	Emitter ChunkEmitter
}

// Run creates/updates a chat node and returns interaction state compatible with bootstrap flow.
func (p *LLMChatNodePipeline) Run(ctx context.Context, in plan.BootstrapIn) (plan.BootstrapOut, error) {
	if p == nil {
		return plan.BootstrapOut{}, fmt.Errorf("testllmChatNode: pipeline is nil")
	}
	if p.LLM == nil {
		return plan.BootstrapOut{}, fmt.Errorf("testllmChatNode: llm client is nil")
	}

	userInput := strings.TrimSpace(in.UserInput)

	initial, ok := ui.BuildChatNode(
		testChatNodeID,
		testChatWorkerKey,
		testChatModelName,
		buildUserHistory(userInput),
		true,
		true,
		"",
	)
	if ok {
		ui.SendUpsertNode(ctx, initial)
	}

	result, err := p.runChatLLM(ctx, userInput)
	if err != nil {
		return plan.BootstrapOut{}, err
	}
	assistant := strings.TrimSpace(result.AssistantMessage)
	needMoreInput := result.NeedMoreInput
	followup := strings.TrimSpace(result.FollowupQuestion)
	if followup == "" {
		followup = assistant
	}
	out := plan.BootstrapOut{
		Result: result,
		BootstrapContext: artifact.BootstrapContext{
			Purpose:   result.Purpose,
			UserInput: userInput,
		}.Normalize(),
		ClientView: &pipelinev1.ClientView{
			Phase: "test_llm_chat_node",
			Content: &pipelinev1.ClientView_LlmResponse{
				LlmResponse: assistant,
			},
		},
	}

	history := buildUserHistory(userInput)
	if needMoreInput {
		node, ok := ui.NeedUserInput(
			testChatNodeID,
			testChatWorkerKey,
			testChatModelName,
			followup,
			history,
		)
		if ok {
			out.UINode = node
			ui.SendUpsertNode(ctx, node)
		}
		return out, nil
	}

	node, ok := ui.Followup(
		testChatNodeID,
		testChatWorkerKey,
		testChatModelName,
		assistant,
		false,
		"",
		history,
	)
	if ok {
		out.UINode = node
		ui.SendUpsertNode(ctx, node)
	}
	return out, nil
}

func (p *LLMChatNodePipeline) runChatLLM(ctx context.Context, userInput string) (artifact.InitPurposeOut, error) {
	payload := map[string]any{
		"user_input": strings.TrimSpace(userInput),
	}
	llmCtx := llm.WithModelSelection(
		llm.WithWorker(ctx, testChatWorkerKey),
		llm.ModelRoleWorker,
		llm.ModelLevelLow,
		"",
		"",
	)
	prompt, err := llmtool.StructuredPromptBuilder(chatPromptSpec)(llmCtx, &llmtool.ToolState{Input: payload}, nil)
	if err != nil {
		return artifact.InitPurposeOut{}, err
	}
	raw, err := p.LLM.GenerateJSONStream(llmCtx, prompt, payload, p.emitChunk)
	if err != nil {
		return artifact.InitPurposeOut{}, err
	}
	var out artifact.InitPurposeOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return artifact.InitPurposeOut{}, fmt.Errorf("testllmChatNode JSON invalid: %w", err)
	}
	out = llmtool.NormalizeNeedInput(llmCtx, out, chatNeedInputAdapter{}, chatNeedInputPolicy, nil)
	return out, nil
}

func buildUserHistory(input string) []ui.ChatMessage {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	return []ui.ChatMessage{
		{
			ID:      "user-" + time.Now().Format("150405.000000000"),
			Role:    ui.RoleUser,
			Content: strings.TrimSpace(input),
		},
	}
}

func (p *LLMChatNodePipeline) emitChunk(chunk string) {
	if p == nil || p.Emitter == nil {
		return
	}
	trimmed := strings.TrimSpace(chunk)
	if trimmed == "" {
		return
	}
	p.Emitter.EmitLLMChunk(trimmed)
}
