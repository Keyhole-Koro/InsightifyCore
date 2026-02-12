package runner

import (
	"context"
	"strings"
)

type userInputKey struct{}

func WithUserInput(ctx context.Context, input string) context.Context {
	return context.WithValue(ctx, userInputKey{}, strings.TrimSpace(input))
}

func UserInputFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(userInputKey{}).(string)
	return strings.TrimSpace(v)
}
