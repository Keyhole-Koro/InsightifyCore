package worker

import (
	"insightify/internal/runner"
	"insightify/internal/safeio"
)

// RunEnvironment abstracts the worker execution environment.
type RunEnvironment interface {
	GetEnv() *runner.Env
	GetOutDir() string
	GetID() string
}

// RunRuntime holds the full runtime environment for a single project run.
type RunRuntime struct {
	ID       string
	RepoName string
	OutDir   string
	Env      *runner.Env
	Cleanup  func()
}

// RunEnvironment interface implementation.
func (r *RunRuntime) GetEnv() *runner.Env { return r.Env }
func (r *RunRuntime) GetOutDir() string   { return r.OutDir }
func (r *RunRuntime) GetID() string       { return r.ID }

type resolvedSources struct {
	Name        string
	SourcePaths []string
	PrimaryPath string
	PrimaryFS   *safeio.SafeFS
}

// NewRunRuntime constructs the full runtime environment for a project run.
func NewRunRuntime(repoName, projectID string) (*RunRuntime, error) {
	// TODO: fully implement
	return &RunRuntime{
		ID:       projectID, // reuse projectID as runID for now or gen new
		RepoName: repoName,
	}, nil
}
