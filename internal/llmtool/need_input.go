package llmtool

import "context"

// NeedInputState is a transport-neutral envelope for interactive follow-up handling.
type NeedInputState struct {
	NeedMoreInput    bool
	AssistantMessage string
	FollowupQuestion string
}

// NeedInputAdapter maps worker-specific output structs to/from NeedInputState.
type NeedInputAdapter[T any] interface {
	Extract(out T) NeedInputState
	Apply(out T, state NeedInputState) T
}

// NeedInputPolicy defines reusable normalization rules.
type NeedInputPolicy struct {
	PreferFollowupAsMessage bool
	RequireAssistantMessage bool
	RequireFollowupQuestion bool
	DefaultAssistantMessage string
	DefaultFollowupQuestion string
}

// NeedInputHooks allows worker-specific adjustments around generic normalization.
type NeedInputHooks[T any] struct {
	BeforeNormalize func(ctx context.Context, out T, state NeedInputState) NeedInputState
	AfterNormalize  func(ctx context.Context, out T, state NeedInputState) NeedInputState
}

// NormalizeNeedInput applies policy-driven normalization for interactive LLM outputs.
func NormalizeNeedInput[T any](
	ctx context.Context,
	out T,
	adapter NeedInputAdapter[T],
	policy NeedInputPolicy,
	hooks *NeedInputHooks[T],
) T {
	if adapter == nil {
		return out
	}

	state := adapter.Extract(out)
	if hooks != nil && hooks.BeforeNormalize != nil {
		state = hooks.BeforeNormalize(ctx, out, state)
	}

	if policy.RequireAssistantMessage && state.AssistantMessage == "" {
		state.AssistantMessage = policy.DefaultAssistantMessage
	}
	if policy.RequireFollowupQuestion && state.FollowupQuestion == "" {
		state.FollowupQuestion = policy.DefaultFollowupQuestion
	}
	if state.NeedMoreInput && policy.PreferFollowupAsMessage && state.FollowupQuestion != "" {
		state.AssistantMessage = state.FollowupQuestion
	}

	if hooks != nil && hooks.AfterNormalize != nil {
		state = hooks.AfterNormalize(ctx, out, state)
	}

	return adapter.Apply(out, state)
}
