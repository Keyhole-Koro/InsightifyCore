package handler

import (
	"context"
	"fmt"
	"log"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	runuc "insightify/internal/gateway/usecase/run"

	"connectrpc.com/connect"
)

func (s *Service) StartRun(ctx context.Context, req *connect.Request[insightifyv1.StartRunRequest]) (*connect.Response[insightifyv1.StartRunResponse], error) {
	projectID, workerKey, userInput, err := s.prepareStartRun(req)
	if err != nil {
		return nil, err
	}
	runID, err := s.launchWorkerRun(projectID, workerKey, userInput)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to start %s: %w", workerKey, err))
	}
	return connect.NewResponse(&insightifyv1.StartRunResponse{RunId: runID}), nil
}

func (s *Service) NeedUserInput(_ context.Context, req *connect.Request[insightifyv1.SubmitRunInputRequest]) (*connect.Response[insightifyv1.SubmitRunInputResponse], error) {
	projectID, runID, userInput, err := s.prepareNeedUserInput(req)
	if err != nil {
		return nil, err
	}
	interactionID, err := s.app.Interaction().SubmitUserInput(projectID, runID, "", userInput)
	if err != nil {
		return nil, err
	}
	_ = interactionID
	return connect.NewResponse(&insightifyv1.SubmitRunInputResponse{RunId: runID}), nil
}

func (s *Service) WatchRun(ctx context.Context, req *connect.Request[insightifyv1.WatchRunRequest], stream *connect.ServerStream[insightifyv1.WatchRunResponse]) error {
	runID := req.Msg.GetRunId()
	eventCh, ok := s.app.RunEventChannel(runID)
	log.Printf("runId %s", runID)
	if !ok {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("run %s not found", runID))
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-eventCh:
			if !ok {
				return nil
			}
			if err := stream.Send(event); err != nil {
				return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send event: %w", err))
			}
			if event.EventType == insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE || event.EventType == insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR {
				return nil
			}
		}
	}
}

// ---------------------------------------------------------------------------
// worker launch wiring
// ---------------------------------------------------------------------------

func (s *Service) launchWorkerRun(projectID, workerKey, userInput string) (string, error) {
	return runuc.LaunchWorkerRun(projectID, workerKey, userInput, s.buildLaunchDeps())
}

// buildLaunchDeps constructs the dependency bag for LaunchWorkerRun.
// Separated from launchWorkerRun so the wiring is scannable at a glance.
func (s *Service) buildLaunchDeps() runuc.LaunchWorkerDeps {
	return runuc.LaunchWorkerDeps{
		GetSession: func(projectID string) (runuc.WorkerSession, bool) {
			sess, ok := s.getProjectState(projectID)
			if !ok || sess.RunCtx == nil {
				return runuc.WorkerSession{}, false
			}
			return runuc.WorkerSession{ProjectID: sess.ProjectID, Env: sess.RunCtx.Env}, true
		},
		NewRunID:        func(workerKey string) string { return fmt.Sprintf("%s-%d", workerKey, time.Now().UnixNano()) },
		MarkRunStarted:  s.app.Interaction().MarkRunStarted,
		MarkRunFinished: s.app.Interaction().MarkRunFinished,
		AllocateEventCh: func(runID string) chan *insightifyv1.WatchRunResponse {
			return s.app.AllocateRunEventChannel(runID, 128)
		},
		CloseEventCh:       func(ch chan *insightifyv1.WatchRunResponse) { close(ch) },
		ClearNeedInput:     s.app.Interaction().Clear,
		ClearRunNode:       s.app.ClearRunNode,
		ScheduleRunCleanup: s.app.ScheduleRunCleanup,
		ExecuteWorkerRun: func(sess runuc.WorkerSession, runID, workerKey, userInput string, eventCh chan *insightifyv1.WatchRunResponse) {
			runuc.ExecuteWorkerRun(sess, runID, workerKey, userInput, eventCh, s.buildExecuteDeps())
		},
	}
}

// buildExecuteDeps constructs the dependency bag for ExecuteWorkerRun.
func (s *Service) buildExecuteDeps() runuc.ExecuteWorkerDeps {
	return runuc.ExecuteWorkerDeps{
		RegisterPendingUserInput: s.app.Interaction().RegisterNeedInput,
		WaitPendingUserInput:     s.app.Interaction().WaitUserInput,
		EmitRunEvent:             s.emitRunEvent,
		SetRunNode:               s.app.SetRunNode,
		ClearRunNode:             s.app.ClearRunNode,
		ToProtoUINode:            toProtoUINode,
	}
}

func (s *Service) emitRunEvent(projectID, runID string, eventCh chan<- *insightifyv1.WatchRunResponse, ev *insightifyv1.WatchRunResponse) {
	if ev == nil {
		return
	}
	if eventCh != nil {
		eventCh <- ev
	}
	s.publishRunEventToChat(projectID, runID, ev)
}
