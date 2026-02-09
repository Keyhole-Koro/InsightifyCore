package artifact

import (
	pipelinev1 "insightify/gen/go/pipeline/v1"
)

// WorkerMeta carries minimal phase info for DAG construction.
type WorkerMeta struct {
	Key         string   `json:"key"`
	Description string   `json:"description"`
	Requires    []string `json:"requires"`
}

// PlanDependenciesIn is the input for the 'worker_DAG' worker.
type PlanDependenciesIn struct {
	RepoPath    string       `json:"repo_path"`
	InitPurpose string       `json:"init_purpose,omitempty"`
	InitRepoURL string       `json:"init_repo_url,omitempty"`
	Workers     []WorkerMeta `json:"workers"`
}

type PlanDependenciesOut struct {
	RuntimeState any                    `json:"artifact"`
	ClientView   *pipelinev1.ClientView `json:"client_view"`
}
