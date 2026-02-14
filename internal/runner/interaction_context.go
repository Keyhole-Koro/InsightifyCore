package runner

import "context"

type runIDContextKey struct{}
type interactionWaiterContextKey struct{}

// InteractionWaiter bridges user interaction input from gateway into runner workers.
type InteractionWaiter interface {
	WaitForInput(ctx context.Context, runID string) (string, error)
	PublishOutput(ctx context.Context, runID, interactionID, message string) error
}

func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, runIDContextKey{}, runID)
}

func RunIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	v, ok := ctx.Value(runIDContextKey{}).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

func WithInteractionWaiter(ctx context.Context, waiter InteractionWaiter) context.Context {
	return context.WithValue(ctx, interactionWaiterContextKey{}, waiter)
}

func InteractionWaiterFromContext(ctx context.Context) (InteractionWaiter, bool) {
	if ctx == nil {
		return nil, false
	}
	v, ok := ctx.Value(interactionWaiterContextKey{}).(InteractionWaiter)
	if !ok || v == nil {
		return nil, false
	}
	return v, true
}
