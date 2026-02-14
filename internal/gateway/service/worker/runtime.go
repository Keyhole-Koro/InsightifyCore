package worker

import (
	"context"
	"os"
	"path/filepath"

	llmclient "insightify/internal/llmClient"
	"insightify/internal/mcp"
	"insightify/internal/runner"
	"insightify/internal/safeio"
	"insightify/internal/scan"
)

// RunEnvironment abstracts the worker execution environment.
type RunEnvironment interface {
	Runtime() runner.Runtime
	GetOutDir() string
	GetID() string
}

// RunRuntime holds the full runtime environment for a single project run.
type RunRuntime struct {
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

// RunEnvironment interface implementation.
func (r *RunRuntime) Runtime() runner.Runtime { return r }
func (r *RunRuntime) GetOutDir() string       { return r.OutDir }
func (r *RunRuntime) GetID() string           { return r.ID }

// runner.Runtime interface implementation.
func (r *RunRuntime) GetRepoFS() *safeio.SafeFS          { return r.RepoFS }
func (r *RunRuntime) GetArtifactFS() *safeio.SafeFS      { return r.ArtifactFS }
func (r *RunRuntime) GetResolver() runner.SpecResolver   { return r.Resolver }
func (r *RunRuntime) GetMCP() *mcp.Registry              { return r.MCP }
func (r *RunRuntime) GetModelSalt() string               { return r.ModelSalt }
func (r *RunRuntime) GetForceFrom() string               { return r.ForceFrom }
func (r *RunRuntime) GetDepsUsage() runner.DepsUsageMode { return r.DepsUsage }
func (r *RunRuntime) GetLLM() llmclient.LLMClient        { return r.LLM }

type resolvedSources struct {
	Name        string
	SourcePaths []string
	PrimaryPath string
	PrimaryFS   *safeio.SafeFS
}

// NewRunRuntime constructs the full runtime environment for a project run.
func NewRunRuntime(repoName, projectID string) (*RunRuntime, error) {
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

	llmCli, modelSalt, err := newGatewayLLMClient(context.Background())
	if err != nil {
		return nil, err
	}

	rt := &RunRuntime{
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
	rt.Resolver = runner.MergeRegistries(
		runner.BuildRegistryArchitecture(rt),
		runner.BuildRegistryCodebase(rt),
		runner.BuildRegistryExternal(rt),
		runner.BuildRegistryPlan(rt),
		runner.BuildRegistryTestWorker(rt),
	)
	return rt, nil
}
