package logctx

import (
	"context"
	"log/slog"

	traceutil "insightify/internal/common/trace"
)

func Info(ctx context.Context, msg string, kv ...any) {
	slog.InfoContext(ctx, msg, append(traceKV(ctx), kv...)...)
}

func Warn(ctx context.Context, msg string, kv ...any) {
	slog.WarnContext(ctx, msg, append(traceKV(ctx), kv...)...)
}

func Error(ctx context.Context, msg string, err error, kv ...any) {
	args := append(traceKV(ctx), kv...)
	if err != nil {
		args = append(args, "error", err.Error())
	}
	slog.ErrorContext(ctx, msg, args...)
}

func traceKV(ctx context.Context) []any {
	if traceID := traceutil.FromContext(ctx); traceID != "" {
		return []any{"trace_id", traceID}
	}
	return nil
}
