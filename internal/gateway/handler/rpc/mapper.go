package rpc

import (
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/service/project"
)

func toProtoProject(e project.Entry) *insightifyv1.Project {
	projectID := strings.TrimSpace(e.State.ProjectID)
	name := strings.TrimSpace(e.State.ProjectName)
	if name == "" {
		name = "Project"
	}
	p := &insightifyv1.Project{
		ProjectId: projectID,
		UserId:    strings.TrimSpace(e.State.UserID),
		Name:      name,
		IsActive:  e.State.IsActive,
	}
	return p
}
