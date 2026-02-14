package testpipe

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"insightify/internal/workers/plan"
)

type scriptedChatLLM struct {
	replies []string
	index   int
}

func (s *scriptedChatLLM) Name() string                { return "scripted-chat-llm" }
func (s *scriptedChatLLM) Close() error                { return nil }
func (s *scriptedChatLLM) CountTokens(text string) int { return len(text) / 4 }
func (s *scriptedChatLLM) TokenCapacity() int          { return 4096 }
func (s *scriptedChatLLM) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	return s.GenerateJSONStream(ctx, prompt, input, nil)
}
func (s *scriptedChatLLM) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	if s.index >= len(s.replies) {
		return json.RawMessage(`{"purpose":"daily conversation","followup_question":"..."}`), nil
	}
	reply := s.replies[s.index]
	s.index++
	out := map[string]any{
		"purpose":           "daily conversation",
		"repo_url":          "",
		"need_more_input":   false,
		"followup_question": reply,
	}
	raw, _ := json.Marshal(out)
	return raw, nil
}

type scriptedInteraction struct {
	inputs   []string
	outputs  []string
	index    int
	outIndex int
}

func (s *scriptedInteraction) WaitForInput(ctx context.Context) (string, error) {
	if s.index >= len(s.inputs) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	in := s.inputs[s.index]
	s.index++
	return in, nil
}

func (s *scriptedInteraction) PublishOutput(_ context.Context, message string) error {
	s.outputs = append(s.outputs, message)
	s.outIndex++
	return nil
}

func TestLLMChatNodePipeline_RunSingleTurnWithoutInteraction(t *testing.T) {
	p := &LLMChatNodePipeline{
		LLM: &scriptedChatLLM{
			replies: []string{"Nice. What did you eat for lunch?"},
		},
	}

	err := p.Run(context.Background(), plan.BootstrapIn{UserInput: "Hi"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestLLMChatNodePipeline_RunLoopsAndStopsOnIdleTimeout(t *testing.T) {
	p := &LLMChatNodePipeline{
		LLM: &scriptedChatLLM{
			replies: []string{
				"Sounds fun. Did anything interesting happen today?",
				"That is great. What are you planning for tomorrow?",
			},
		},
		Interaction: &scriptedInteraction{
			inputs: []string{"Pretty normal day"},
		},
		IdleTimeout: 20 * time.Millisecond,
		MaxTurns:    5,
	}

	err := p.Run(context.Background(), plan.BootstrapIn{UserInput: "Hey"})
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v", err)
	}
	if len(p.Interaction.(*scriptedInteraction).outputs) == 0 {
		t.Fatalf("expected at least one published output")
	}
}

func TestLLMChatNodePipeline_SendsOpeningMessageWhenInputEmpty(t *testing.T) {
	interaction := &scriptedInteraction{
		inputs: []string{"I had a good day"},
	}
	p := &LLMChatNodePipeline{
		LLM: &scriptedChatLLM{
			replies: []string{"Nice. What made it good?"},
		},
		Interaction: interaction,
		IdleTimeout: 20 * time.Millisecond,
		MaxTurns:    3,
	}

	err := p.Run(context.Background(), plan.BootstrapIn{})
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v", err)
	}
	if len(interaction.outputs) < 2 {
		t.Fatalf("outputs len = %d, want >= 2", len(interaction.outputs))
	}
	if interaction.outputs[0] != defaultOpeningPrompt {
		t.Fatalf("opening output = %q, want %q", interaction.outputs[0], defaultOpeningPrompt)
	}
}
