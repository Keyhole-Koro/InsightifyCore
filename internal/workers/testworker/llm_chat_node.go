package testpipe

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
	"insightify/internal/workers/plan"
)

const (
	testChatNodeID    = "test-llm-chat-node"
	testChatWorkerKey = "testllmChatNode"
	testChatModelName = "Low"
)

var chatPromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Carry out a casual daily conversation with the user and return one natural reply.",
	Background:   "This worker powers an interactive chat node and returns one assistant turn per call.",
	OutputFields: llmtool.MustFieldsFromStruct(artifact.InitPurposeOut{}),
	Constraints: []string{
		"Return a concise and natural followup_question as the assistant reply.",
	},
	Rules: []string{
		"Prioritize the latest user_input.",
		"Ask one clear follow-up question or reply naturally in one turn.",
	},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent())

// LLMChatNodePipeline is an interactive chat worker backed by LLM.
type LLMChatNodePipeline struct {
	LLM llmclient.LLMClient
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
	payload := map[string]any{
		"user_input": userInput,
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
		return plan.BootstrapOut{}, err
	}
	raw, err := p.LLM.GenerateJSONStream(llmCtx, prompt, payload, nil)
	if err != nil {
		return plan.BootstrapOut{}, err
	}
	var result artifact.InitPurposeOut
	if err := json.Unmarshal(raw, &result); err != nil {
		return plan.BootstrapOut{}, fmt.Errorf("testllmChatNode JSON invalid: %w", err)
	}
	return plan.BootstrapOut{
		Result: result,
		BootstrapContext: artifact.BootstrapContext{
			Purpose:   result.Purpose,
			UserInput: userInput,
		}.Normalize(),
		ClientView: &workerv1.ClientView{
			Phase: "test_llm_chat_node",
			Content: &workerv1.ClientView_LlmResponse{
				LlmResponse: strings.TrimSpace(result.FollowupQuestion),
			},
		},
	}, nil
}
