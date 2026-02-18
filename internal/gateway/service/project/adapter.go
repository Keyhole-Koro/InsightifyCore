package project

import (
	"context"

	gatewayworker "insightify/internal/gateway/service/worker"
	runtimepkg "insightify/internal/workerruntime"
)

// AsProjectReader returns an adapter that satisfies worker.ProjectReader.
func (s *Service) AsProjectReader() gatewayworker.ProjectReader {
	return &projectReaderAdapter{svc: s}
}

type projectReaderAdapter struct {
	svc *Service
}

func (a *projectReaderAdapter) GetEntry(projectID string) (gatewayworker.ProjectView, bool) {
	e, ok := a.svc.get(context.Background(), projectID)
	if !ok {
		return gatewayworker.ProjectView{}, false
	}
	return gatewayworker.ProjectView{
		ProjectID: e.State.ProjectID,
		RunCtx:    e.RunCtx,
	}, true
}

func (a *projectReaderAdapter) EnsureRunContext(projectID string) (*runtimepkg.ProjectRuntime, error) {
	return a.svc.EnsureRunContext(projectID)
}
