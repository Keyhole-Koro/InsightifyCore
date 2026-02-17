package testpipe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	llmclient "insightify/internal/llmclient"
	"insightify/internal/llmtool"
	"insightify/internal/workers/plan"
)

const (
	testChatNodeID       = "test-llm-chat-node"
	testChatWorkerKey    = "testllmChatNode"
	testChatModelName    = "Low"
	defaultChatMaxTurns  = 8
	defaultIdleTimeout   = 30 * time.Second
	defaultOpeningPrompt = "Hi! How has your day been so far?"
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
		"Use history to keep the conversation coherent.",
		"Ask one clear follow-up question or reply naturally in one turn.",
	},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent())

type Interaction interface {
	WaitForInput(ctx context.Context) (string, error)
	PublishOutput(ctx context.Context, message string) error
}

type chatTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LLMChatNodePipeline is an interactive chat worker backed by LLM.
type LLMChatNodePipeline struct {
	LLM         llmclient.LLMClient
	Interaction Interaction
	MaxTurns    int
	IdleTimeout time.Duration
}

// Run creates/updates a chat node and handles interactive conversation turns.
func (p *LLMChatNodePipeline) Run(ctx context.Context, in plan.BootstrapIn) error {
	if p == nil {
		return fmt.Errorf("testllmChatNode: pipeline is nil")
	}
	if p.LLM == nil {
		return fmt.Errorf("testllmChatNode: llm client is nil")
	}

	maxTurns := p.MaxTurns
	if maxTurns <= 0 {
		maxTurns = defaultChatMaxTurns
	}

	idleTimeout := p.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = defaultIdleTimeout
	}

	userInput := strings.TrimSpace(in.UserInput)
	waitNextInput := func() (string, error) {
		if p.Interaction == nil {
			return "", nil
		}
		waitCtx, cancel := context.WithTimeout(ctx, idleTimeout)
		nextInput, waitErr := p.Interaction.WaitForInput(waitCtx)
		cancel()
		if waitErr != nil {
			return "", waitErr
		}
		return strings.TrimSpace(nextInput), nil
	}
	if userInput == "" {
		if p.Interaction == nil {
			userInput = defaultOpeningPrompt
		} else {
			if err := p.Interaction.PublishOutput(ctx, defaultOpeningPrompt); err != nil {
				return err
			}
			nextInput, waitErr := waitNextInput()
			if waitErr != nil {
				if errors.Is(waitErr, context.DeadlineExceeded) || errors.Is(waitErr, context.Canceled) {
					return nil
				}
				return waitErr
			}
			if nextInput == "" {
				return nil
			}
			userInput = nextInput
		}
	}

	history := make([]chatTurn, 0, maxTurns*2)
	turn := 0

	for {
		if turn >= maxTurns {
			break
		}
		turn++

		history = append(history, chatTurn{Role: "user", Content: userInput})
		reply, err := p.generateReply(ctx, userInput, history)
		if err != nil {
			return err
		}
		reply = strings.TrimSpace(reply)
		if reply == "" {
			break
		}

		if p.Interaction != nil {
			if err := p.Interaction.PublishOutput(ctx, reply); err != nil {
				return err
			}
		}

		history = append(history, chatTurn{Role: "assistant", Content: reply})
		if p.Interaction == nil {
			return nil
		}

		nextInput, waitErr := waitNextInput()
		if waitErr != nil {
			if errors.Is(waitErr, context.DeadlineExceeded) || errors.Is(waitErr, context.Canceled) {
				break
			}
			return waitErr
		}
		if nextInput == "" {
			break
		}
		userInput = nextInput
	}

	return nil
}

func (p *LLMChatNodePipeline) generateReply(ctx context.Context, userInput string, history []chatTurn) (string, error) {
	payload := map[string]any{
		"user_input": strings.TrimSpace(userInput),
		"history":    history,
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
		return "", err
	}
	raw, err := p.LLM.GenerateJSONStream(llmCtx, prompt, payload, nil)
	if err != nil {
		return "", err
	}
	var result artifact.InitPurposeOut
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("testllmChatNode JSON invalid: %w", err)
	}
	return strings.TrimSpace(result.FollowupQuestion), nil
}
