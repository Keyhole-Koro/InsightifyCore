package project

import (
	gatewayrun "insightify/internal/gateway/run"
	"insightify/internal/gateway/runtime"
)

// AsProjectReader returns an adapter that satisfies run.ProjectReader.
func (s *Service) AsProjectReader() gatewayrun.ProjectReader {
	return &projectReaderAdapter{svc: s}
}

type projectReaderAdapter struct {
	svc *Service
}

func (a *projectReaderAdapter) GetEntry(projectID string) (gatewayrun.ProjectView, bool) {
	e, ok := a.svc.get(projectID)
	if !ok {
		return gatewayrun.ProjectView{}, false
	}
	return gatewayrun.ProjectView{
		ProjectID: e.State.ProjectID,
		RunCtx:    e.RunCtx,
	}, true
}

func (a *projectReaderAdapter) EnsureRunContext(projectID string) (runtime.RunEnvironment, error) {
	return a.svc.EnsureRunContext(projectID)
}
