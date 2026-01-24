package runner

import (
	"context"
	"strings"
	"testing"

	"insightify/internal/safeio"
)

type testArtifact struct {
	Value string `json:"value"`
}

func newTestEnv(t *testing.T) *Env {
	t.Helper()
	dir := t.TempDir()
	fs, err := safeio.NewSafeFS(dir)
	if err != nil {
		t.Fatalf("safe fs: %v", err)
	}
	return &Env{
		Repo:       "test",
		RepoRoot:   dir,
		OutDir:     dir,
		MaxNext:    1,
		RepoFS:     fs,
		ArtifactFS: fs,
	}
}

func buildTestRegistry(env *Env, runs map[string]int) map[string]PhaseSpec {
	reg := map[string]PhaseSpec{}
	reg["m0"] = PhaseSpec{
		Key:  "m0",
		File: "m0.json",
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			return nil, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			runs["m0"]++
			return PhaseOutput{RuntimeState: testArtifact{Value: "m0"}, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return "fp0"
		},
		Strategy: jsonStrategy{},
	}
	reg["m1"] = PhaseSpec{
		Key:      "m1",
		File:     "m1.json",
		Requires: []string{"m0"},
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var prev testArtifact
			if err := deps.Artifact("m0", &prev); err != nil {
				return nil, err
			}
			return testArtifact{Value: prev.Value + "+m1"}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			runs["m1"]++
			return PhaseOutput{RuntimeState: in, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return "fp1"
		},
		Strategy: jsonStrategy{},
	}
	reg["m2"] = PhaseSpec{
		Key:      "m2",
		File:     "m2.json",
		Requires: []string{"m1"},
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var prev testArtifact
			if err := deps.Artifact("m1", &prev); err != nil {
				return nil, err
			}
			return testArtifact{Value: prev.Value + "+m2"}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			runs["m2"]++
			return PhaseOutput{RuntimeState: in, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return "fp2"
		},
		Strategy: jsonStrategy{},
	}
	return reg
}

func TestExecutePhaseBuildsDependencies(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	runs := map[string]int{}
	reg := buildTestRegistry(env, runs)
	env.Resolver = MergeRegistries(reg)

	if err := ExecutePhase(ctx, reg["m2"], env); err != nil {
		t.Fatalf("execute m2: %v", err)
	}
	if runs["m0"] != 1 || runs["m1"] != 1 || runs["m2"] != 1 {
		t.Fatalf("unexpected run counts: %+v", runs)
	}
	m2, err := Artifact[testArtifact](env, "m2")
	if err != nil {
		t.Fatalf("read m2 artifact: %v", err)
	}
	if m2.Value != "m0+m1+m2" {
		t.Fatalf("unexpected m2 artifact: %+v", m2)
	}
}

func TestExecutePhaseSkipsWhenArtifactsExist(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	runs := map[string]int{}
	reg := buildTestRegistry(env, runs)
	env.Resolver = MergeRegistries(reg)

	if err := ExecutePhase(ctx, reg["m2"], env); err != nil {
		t.Fatalf("initial execute: %v", err)
	}
	runs["m0"], runs["m1"], runs["m2"] = 0, 0, 0
	if err := ExecutePhase(ctx, reg["m2"], env); err != nil {
		t.Fatalf("cache execute: %v", err)
	}
	if runs["m0"] != 0 || runs["m1"] != 0 || runs["m2"] != 0 {
		t.Fatalf("expected no runs on cache hit, got %+v", runs)
	}
}

func TestExecutePhaseDetectsCycles(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	reg := map[string]PhaseSpec{
		"a": {
			Key:        "a",
			File:       "a.json",
			Requires:   []string{"b"},
			BuildInput: func(ctx context.Context, deps Deps) (any, error) { return nil, nil },
			Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
				return PhaseOutput{RuntimeState: testArtifact{Value: "a"}, ClientView: nil}, nil
			},
			Fingerprint: func(in any, env *Env) string { return "a" },
			Strategy:    jsonStrategy{},
		},
		"b": {
			Key:        "b",
			File:       "b.json",
			Requires:   []string{"a"},
			BuildInput: func(ctx context.Context, deps Deps) (any, error) { return nil, nil },
			Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
				return PhaseOutput{RuntimeState: testArtifact{Value: "b"}, ClientView: nil}, nil
			},
			Fingerprint: func(in any, env *Env) string { return "b" },
			Strategy:    jsonStrategy{},
		},
	}
	env.Resolver = MergeRegistries(reg)

	err := ExecutePhase(ctx, reg["a"], env)
	if err == nil || !strings.Contains(err.Error(), "cyclic") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestExecutePhaseFailsOnUnusedRequires(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	reg := map[string]PhaseSpec{
		"a": {
			Key:        "a",
			File:       "a.json",
			Requires:   []string{"b"},
			BuildInput: func(ctx context.Context, deps Deps) (any, error) { return nil, nil },
			Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
				return PhaseOutput{RuntimeState: testArtifact{Value: "a"}, ClientView: nil}, nil
			},
			Fingerprint: func(in any, env *Env) string { return "a" },
			Strategy:    jsonStrategy{},
		},
		"b": {
			Key:        "b",
			File:       "b.json",
			BuildInput: func(ctx context.Context, deps Deps) (any, error) { return nil, nil },
			Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
				return PhaseOutput{RuntimeState: testArtifact{Value: "b"}, ClientView: nil}, nil
			},
			Fingerprint: func(in any, env *Env) string { return "b" },
			Strategy:    jsonStrategy{},
		},
	}
	env.Resolver = MergeRegistries(reg)

	err := ExecutePhase(ctx, reg["a"], env)
	if err == nil || !strings.Contains(err.Error(), "declared but did not use") {
		t.Fatalf("expected unused requires error, got %v", err)
	}
}

func TestArtifactUsesResolverFile(t *testing.T) {
	env := newTestEnv(t)
	env.Resolver = MergeRegistries(map[string]PhaseSpec{
		"x": {Key: "x", File: "custom.json"},
	})
	WriteJSON(env.OutDir, "custom.json", testArtifact{Value: "hello"})

	got, err := Artifact[testArtifact](env, "x")
	if err != nil {
		t.Fatalf("artifact read: %v", err)
	}
	if got.Value != "hello" {
		t.Fatalf("unexpected artifact value: %+v", got)
	}
}
