package handler

import (
	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/usecase/needinput"
	projectuc "insightify/internal/gateway/usecase/project"
	runuc "insightify/internal/gateway/usecase/run"

	"connectrpc.com/connect"
)

func (s *Service) prepareInitRun(req *connect.Request[insightifyv1.InitRunRequest]) projectuc.InitRunInput {
	s.app.EnsureProjectStoreLoaded()
	return projectuc.PrepareInitRun(req.Msg.GetUserId(), req.Msg.GetRepoUrl(), req.Msg.GetProjectId())
}

func (s *Service) prepareStartRun(req *connect.Request[insightifyv1.StartRunRequest]) (projectID, workerKey, userInput string, err error) {
	s.app.EnsureProjectStoreLoaded()
	in, err := runuc.PrepareStartRun(
		req.Msg.GetProjectId(),
		req.Msg.GetPipelineId(),
		req.Msg.GetParams()["user_input"],
		runuc.StartRunDeps{
			ProjectExists: func(projectID string) bool {
				_, ok := s.getProjectState(projectID)
				return ok
			},
			EnsureProjectRunContext: func(projectID string) error {
				_, err := s.ensureProjectRunContext(projectID)
				return err
			},
		},
	)
	if err != nil {
		return "", "", "", err
	}
	return in.ProjectID, in.WorkerKey, in.UserInput, nil
}

func (s *Service) prepareNeedUserInput(req *connect.Request[insightifyv1.SubmitRunInputRequest]) (projectID, runID, userInput string, err error) {
	s.app.EnsureProjectStoreLoaded()
	in, err := needinput.PrepareSubmitRunInput(
		req.Msg.GetProjectId(),
		req.Msg.GetRunId(),
		req.Msg.GetInput(),
		s.app.ActiveRunID,
	)
	if err != nil {
		return "", "", "", err
	}
	return in.ProjectID, in.RunID, in.Input, nil
}

func (s *Service) prepareSendMessage(req *connect.Request[insightifyv1.SendMessageRequest]) (projectID, runID, interactionID, input string, err error) {
	s.app.EnsureProjectStoreLoaded()
	in, err := needinput.PrepareSendMessage(
		req.Msg.GetProjectId(),
		req.Msg.GetRunId(),
		req.Msg.GetInteractionId(),
		req.Msg.GetInput(),
	)
	if err != nil {
		return "", "", "", "", err
	}
	return in.ProjectID, in.RunID, in.InteractionID, in.Input, nil
}
