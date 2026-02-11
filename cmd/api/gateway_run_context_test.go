package main

import (
	"testing"

	"insightify/internal/scan"
)

func TestNewRunContextIncludesTestWorkerRegistry(t *testing.T) {
	scan.SetReposDir(t.TempDir())

	ctx, err := NewRunContext("", "test-session")
	if err != nil {
		t.Fatalf("NewRunContext() error = %v", err)
	}
	t.Cleanup(func() {
		if ctx != nil && ctx.Cleanup != nil {
			ctx.Cleanup()
		}
	})
	if ctx == nil || ctx.Env == nil || ctx.Env.Resolver == nil {
		t.Fatalf("run context/env/resolver must not be nil")
	}

	if _, ok := ctx.Env.Resolver.Get("testllmChar"); !ok {
		t.Fatalf("expected testllmChar to be registered in resolver")
	}
}
