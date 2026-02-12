package project

import (
	"time"

	"connectrpc.com/connect"
	insightifyv1 "insightify/gen/go/insightify/v1"
)

// InitRunEntry handles request preprocessing and usecase delegation for InitRun.
type InitRunEntry struct {
	deps Deps
	now  func() time.Time
}

func NewInitRunEntry(deps Deps, now func() time.Time) *InitRunEntry {
	if now == nil {
		now = time.Now
	}
	return &InitRunEntry{deps: deps, now: now}
}

func (e *InitRunEntry) Handle(req *connect.Request[insightifyv1.InitRunRequest]) (*connect.Response[insightifyv1.InitRunResponse], error) {
	in := e.prepare(req)
	updated, err := InitRun(in, e.now(), e.deps)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&insightifyv1.InitRunResponse{
		RepoName:       updated.Repo,
		BootstrapRunId: "",
		ProjectId:      updated.ProjectID,
	}), nil
}

func (e *InitRunEntry) prepare(req *connect.Request[insightifyv1.InitRunRequest]) InitRunInput {
	if req == nil || req.Msg == nil {
		return PrepareInitRun("", "", "")
	}
	return PrepareInitRun(req.Msg.GetUserId(), req.Msg.GetRepoUrl(), req.Msg.GetProjectId())
}
