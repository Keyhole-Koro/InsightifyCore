package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/artifact"
	"insightify/internal/gateway/runtime"
)

func toProtoProject(e entry) *insightifyv1.Project {
	bc := readBootstrapContext(e)
	projectID := strings.TrimSpace(e.State.ProjectID)
	name := strings.TrimSpace(e.State.ProjectName)
	if name == "" {
		name = "Project"
	}
	return &insightifyv1.Project{
		ProjectId: projectID,
		UserId:    strings.TrimSpace(e.State.UserID),
		Name:      name,
		RepoUrl:   strings.TrimSpace(bc.RepoURL),
		Purpose:   strings.TrimSpace(bc.Purpose),
		RepoName:  strings.TrimSpace(e.State.Repo),
		IsActive:  e.State.IsActive,
	}
}

func readBootstrapContext(e entry) artifact.BootstrapContext {
	runCtx, ok := e.RunCtx.(*runtime.RunContext)
	if !ok || runCtx == nil {
		return artifact.BootstrapContext{}
	}
	path := filepath.Join(runCtx.OutDir, "bootstrap.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return artifact.BootstrapContext{}
	}
	var raw struct {
		BootstrapContext artifact.BootstrapContext `json:"bootstrap_context"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return artifact.BootstrapContext{}
	}
	return raw.BootstrapContext.Normalize()
}
