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

func buildTestRegistry(env *Env, runs map[string]int) map[string]WorkerSpec {
	reg := map[string]WorkerSpec{}
	reg["m0"] = WorkerSpec{
		Key: "m0",
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			return nil, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			runs["m0"]++
			return WorkerOutput{RuntimeState: testArtifact{Value: "m0"}, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return "fp0"
		},
		Strategy: jsonStrategy{},
	}
	reg["m1"] = WorkerSpec{
		Key:      "m1",
		Requires: []string{"m0"},
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var prev testArtifact
			if err := deps.Artifact("m0", &prev); err != nil {
				return nil, err
			}
			return testArtifact{Value: prev.Value + "+m1"}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			runs["m1"]++
			return WorkerOutput{RuntimeState: in, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return "fp1"
		},
		Strategy: jsonStrategy{},
	}
	reg["m2"] = WorkerSpec{
		Key:      "m2",
		Requires: []string{"m1"},
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var prev testArtifact
			if err := deps.Artifact("m1", &prev); err != nil {
				return nil, err
			}
			return testArtifact{Value: prev.Value + "+m2"}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			runs["m2"]++
			return WorkerOutput{RuntimeState: in, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return "fp2"
		},
		Strategy: jsonStrategy{},
	}
	return reg
}

func TestExecuteWorkerBuildsDependencies(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	runs := map[string]int{}
	reg := buildTestRegistry(env, runs)
	env.Resolver = MergeRegistries(reg)

	if err := ExecuteWorker(ctx, reg["m2"], env); err != nil {
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

func TestExecuteWorkerSkipsWhenArtifactsExist(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	runs := map[string]int{}
	reg := buildTestRegistry(env, runs)
	env.Resolver = MergeRegistries(reg)

	if err := ExecuteWorker(ctx, reg["m2"], env); err != nil {
		t.Fatalf("initial execute: %v", err)
	}
	runs["m0"], runs["m1"], runs["m2"] = 0, 0, 0
	if err := ExecuteWorker(ctx, reg["m2"], env); err != nil {
		t.Fatalf("cache execute: %v", err)
	}
	if runs["m0"] != 0 || runs["m1"] != 0 || runs["m2"] != 0 {
		t.Fatalf("expected no runs on cache hit, got %+v", runs)
	}
}

func TestExecuteWorkerDetectsCycles(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	reg := map[string]WorkerSpec{
		"a": {
			Key:        "a",
			Requires:   []string{"b"},
			BuildInput: func(ctx context.Context, deps Deps) (any, error) { return nil, nil },
			Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
				return WorkerOutput{RuntimeState: testArtifact{Value: "a"}, ClientView: nil}, nil
			},
			Fingerprint: func(in any, env *Env) string { return "a" },
			Strategy:    jsonStrategy{},
		},
		"b": {
			Key:        "b",
			Requires:   []string{"a"},
			BuildInput: func(ctx context.Context, deps Deps) (any, error) { return nil, nil },
			Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
				return WorkerOutput{RuntimeState: testArtifact{Value: "b"}, ClientView: nil}, nil
			},
			Fingerprint: func(in any, env *Env) string { return "b" },
			Strategy:    jsonStrategy{},
		},
	}
	env.Resolver = MergeRegistries(reg)

	err := ExecuteWorker(ctx, reg["a"], env)
	if err == nil || !strings.Contains(err.Error(), "cyclic") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestExecuteWorkerFailsOnUnusedRequires(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	reg := map[string]WorkerSpec{
		"a": {
			Key:        "a",
			Requires:   []string{"b"},
			BuildInput: func(ctx context.Context, deps Deps) (any, error) { return nil, nil },
			Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
				return WorkerOutput{RuntimeState: testArtifact{Value: "a"}, ClientView: nil}, nil
			},
			Fingerprint: func(in any, env *Env) string { return "a" },
			Strategy:    jsonStrategy{},
		},
		"b": {
			Key:        "b",
			BuildInput: func(ctx context.Context, deps Deps) (any, error) { return nil, nil },
			Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
				return WorkerOutput{RuntimeState: testArtifact{Value: "b"}, ClientView: nil}, nil
			},
			Fingerprint: func(in any, env *Env) string { return "b" },
			Strategy:    jsonStrategy{},
		},
	}
	env.Resolver = MergeRegistries(reg)

	err := ExecuteWorker(ctx, reg["a"], env)
	if err == nil || !strings.Contains(err.Error(), "declared but did not use") {
		t.Fatalf("expected unused requires error, got %v", err)
	}
}

func TestArtifactUsesKeyJSON(t *testing.T) {
	env := newTestEnv(t)
	env.Resolver = MergeRegistries(map[string]WorkerSpec{
		"x": {Key: "x"},
	})
	WriteJSON(env.OutDir, "x.json", testArtifact{Value: "hello"})

	got, err := Artifact[testArtifact](env, "x")
	if err != nil {
		t.Fatalf("artifact read: %v", err)
	}
	if got.Value != "hello" {
		t.Fatalf("unexpected artifact value: %+v", got)
	}
}
