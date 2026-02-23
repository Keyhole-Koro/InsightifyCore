package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"os/signal"
	"syscall"
	"time"

	"insightify/internal/gateway/app"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(newLogWriter(), &slog.HandlerOptions{})))

	a, err := app.New()
	if err != nil {
		slog.Error("Failed to initialize app", "error", err.Error())
		os.Exit(1)
	}

	go func() {
		if err := a.Start(); err != nil {
			slog.Error("Server error", "error", err.Error())
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err.Error())
		os.Exit(1)
	}

	slog.Info("Server exiting")
}

func newLogWriter() io.Writer {
	if err := os.MkdirAll("logs", 0o755); err != nil {
		return os.Stdout
	}
	logPath := filepath.Join("logs", "core.jsonl")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return os.Stdout
	}
	return io.MultiWriter(os.Stdout, f)
}
