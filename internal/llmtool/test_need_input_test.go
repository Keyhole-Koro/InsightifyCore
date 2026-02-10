package llmtool

import (
	"context"
	"testing"
)

type needInputFixture struct {
	NeedMoreInput    bool
	AssistantMessage string
	FollowupQuestion string
}

type fixtureAdapter struct{}

func (fixtureAdapter) Extract(out needInputFixture) NeedInputState {
	return NeedInputState{
		NeedMoreInput:    out.NeedMoreInput,
		AssistantMessage: out.AssistantMessage,
		FollowupQuestion: out.FollowupQuestion,
	}
}

func (fixtureAdapter) Apply(out needInputFixture, state NeedInputState) needInputFixture {
	out.NeedMoreInput = state.NeedMoreInput
	out.AssistantMessage = state.AssistantMessage
	out.FollowupQuestion = state.FollowupQuestion
	return out
}

func TestNormalizeNeedInput_PolicyDefaults(t *testing.T) {
	in := needInputFixture{NeedMoreInput: true}

	out := NormalizeNeedInput(context.Background(), in, fixtureAdapter{}, NeedInputPolicy{
		RequireAssistantMessage: true,
		RequireFollowupQuestion: true,
		DefaultAssistantMessage: "ask more",
		DefaultFollowupQuestion: "which repo?",
	}, nil)

	if out.AssistantMessage != "ask more" {
		t.Fatalf("assistant default not applied: %q", out.AssistantMessage)
	}
	if out.FollowupQuestion != "which repo?" {
		t.Fatalf("followup default not applied: %q", out.FollowupQuestion)
	}
}

func TestNormalizeNeedInput_PreferFollowupAsMessage(t *testing.T) {
	in := needInputFixture{
		NeedMoreInput:    true,
		AssistantMessage: "generic",
		FollowupQuestion: "please share repo url",
	}
	out := NormalizeNeedInput(context.Background(), in, fixtureAdapter{}, NeedInputPolicy{
		PreferFollowupAsMessage: true,
	}, nil)

	if out.AssistantMessage != in.FollowupQuestion {
		t.Fatalf("assistant_message should be overwritten by followup_question: %q", out.AssistantMessage)
	}
}

func TestNormalizeNeedInput_Hooks(t *testing.T) {
	in := needInputFixture{AssistantMessage: "hello", FollowupQuestion: "next?"}
	out := NormalizeNeedInput(context.Background(), in, fixtureAdapter{}, NeedInputPolicy{}, &NeedInputHooks[needInputFixture]{
		BeforeNormalize: func(ctx context.Context, out needInputFixture, state NeedInputState) NeedInputState {
			state.NeedMoreInput = true
			return state
		},
		AfterNormalize: func(ctx context.Context, out needInputFixture, state NeedInputState) NeedInputState {
			state.AssistantMessage = "adjusted"
			return state
		},
	})

	if !out.NeedMoreInput {
		t.Fatalf("expected hook to set need_more_input")
	}
	if out.AssistantMessage != "adjusted" {
		t.Fatalf("expected after hook override, got %q", out.AssistantMessage)
	}
}
