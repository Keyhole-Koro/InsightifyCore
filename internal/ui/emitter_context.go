package ui

import "context"

type uiEmitterKey struct{}

func WithEmitter(ctx context.Context, emitter Emitter) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, uiEmitterKey{}, emitter)
}

func EmitterFrom(ctx context.Context) Emitter {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(uiEmitterKey{}).(Emitter); ok {
		return v
	}
	return nil
}
