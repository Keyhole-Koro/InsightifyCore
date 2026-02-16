package rpc

import (
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/entity"
	"insightify/internal/gateway/service/project"
)

func toProtoProject(e project.Entry) *insightifyv1.Project {
	projectID := strings.TrimSpace(e.State.ProjectID)
	name := strings.TrimSpace(e.State.ProjectName)
	if name == "" {
		name = "Project"
	}
	var artifacts []*insightifyv1.Artifact
	for _, a := range e.Artifacts {
		artifacts = append(artifacts, &insightifyv1.Artifact{
			Id:        a.ID,
			RunId:     a.RunID,
			Path:      a.Path,
			Url:       a.URL,
			CreatedAt: a.CreatedAt.Format(time.RFC3339),
		})
	}

	p := &insightifyv1.Project{
		ProjectId: projectID,
		UserId:    entity.NormalizeUserID(e.State.UserID).String(),
		Name:      name,
		IsActive:  e.State.IsActive,
		Artifacts: artifacts,
	}
	return p
}
