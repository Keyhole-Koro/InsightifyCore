package artifact

import (
	pipelinev1 "insightify/gen/go/pipeline/v1"
)

// PhaseMeta carries minimal phase info for DAG construction.
type PhaseMeta struct {
	Key         string   `json:"key"`
	Description string   `json:"description"`
	Requires    []string `json:"requires"`
}

// PlanDependenciesIn is the input for the 'phase_DAG' phase.
type PlanDependenciesIn struct {
	RepoPath string      `json:"repo_path"`
	Phases   []PhaseMeta `json:"phases"`
}

type PlanDependenciesOut struct {
	RuntimeState any                    `json:"artifact"`
	ClientView   *pipelinev1.ClientView `json:"client_view"`
}
