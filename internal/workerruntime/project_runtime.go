package runtime

import (
	"context"
	"os"
	"path/filepath"

	llmclient "insightify/internal/llmClient"
	"insightify/internal/mcp"
	"insightify/internal/runner"
	"insightify/internal/common/safeio"
	"insightify/internal/common/scan"
)

// ProjectRuntime holds long-lived runtime dependencies for a project.
type ProjectRuntime struct {
	ID       string
	RepoName string
	OutDir   string

	RepoFS     *safeio.SafeFS
	ArtifactFS *safeio.SafeFS
	Resolver   runner.SpecResolver
	MCP        *mcp.Registry
	ModelSalt  string
	ForceFrom  string
	DepsUsage  runner.DepsUsageMode
	LLM        llmclient.LLMClient

	Cleanup func()
}

// ExecutionOptions controls per-execution runtime overrides.
type ExecutionOptions struct {
	OutDir    string
	ForceFrom string
	DepsUsage runner.DepsUsageMode
}

// ExecutionRuntime provides a runner.Runtime view for a single execution.
type ExecutionRuntime struct {
	project   *ProjectRuntime
	outDir    string
	forceFrom string
	depsUsage runner.DepsUsageMode
	artifact  runner.ArtifactAccess
}

// ProjectRuntime interface-style accessors.
func (r *ProjectRuntime) Runtime() runner.Runtime {
	return r.NewExecutionRuntime(ExecutionOptions{})
}
func (r *ProjectRuntime) GetOutDir() string { return r.OutDir }
func (r *ProjectRuntime) GetID() string     { return r.ID }

// NewExecutionRuntime builds a per-execution runtime from project defaults.
func (r *ProjectRuntime) NewExecutionRuntime(opts ExecutionOptions) *ExecutionRuntime {
	outDir := opts.OutDir
	if outDir == "" {
		outDir = r.OutDir
	}
	exec := &ExecutionRuntime{
		project:   r,
		outDir:    outDir,
		forceFrom: opts.ForceFrom,
		depsUsage: opts.DepsUsage,
	}
	exec.artifact = newLocalArtifactAccess(exec)
	return exec
}

// runner.Runtime interface implementation.
func (r *ExecutionRuntime) GetOutDir() string                  { return r.outDir }
func (r *ExecutionRuntime) GetRepoFS() *safeio.SafeFS          { return r.project.RepoFS }
func (r *ExecutionRuntime) Artifacts() runner.ArtifactAccess   { return r.artifact }
func (r *ExecutionRuntime) GetResolver() runner.SpecResolver   { return r.project.Resolver }
func (r *ExecutionRuntime) GetMCP() *mcp.Registry              { return r.project.MCP }
func (r *ExecutionRuntime) GetModelSalt() string               { return r.project.ModelSalt }
func (r *ExecutionRuntime) GetForceFrom() string               { return r.forceFrom }
func (r *ExecutionRuntime) GetDepsUsage() runner.DepsUsageMode { return r.depsUsage }
func (r *ExecutionRuntime) GetLLM() llmclient.LLMClient        { return r.project.LLM }

// NewProjectRuntime constructs the full runtime environment for a project.
func NewProjectRuntime(repoName, projectID string) (*ProjectRuntime, error) {
	repoFS := safeio.Default()
	if repoFS == nil {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		repoFS, err = safeio.NewSafeFS(cwd)
		if err != nil {
			return nil, err
		}
	}

	outDir := filepath.Join("tmp", "artifacts", projectID)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	artifactFS, err := safeio.NewSafeFS(".")
	if err != nil {
		return nil, err
	}

	llmCli, modelSalt, err := newRuntimeLLMClient(context.Background())
	if err != nil {
		return nil, err
	}

	rt := &ProjectRuntime{
		ID:         projectID,
		RepoName:   repoName,
		OutDir:     outDir,
		RepoFS:     repoFS,
		ArtifactFS: artifactFS,
		LLM:        llmCli,
		ModelSalt:  modelSalt,
	}
	rt.Cleanup = func() {
		if rt.LLM != nil {
			_ = rt.LLM.Close()
		}
	}
	rt.MCP = mcp.NewRegistry()
	mcp.RegisterDefaultTools(rt.MCP, mcp.Host{RepoRoot: repoFS.Root(), ReposRoot: scan.ReposDir(), RepoFS: repoFS, ArtifactFS: artifactFS})
	runtimeView := rt.Runtime()
	rt.Resolver = runner.BuildAllRegistries(runtimeView)
	return rt, nil
}
