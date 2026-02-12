package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
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
	interactionID, err := s.app.SubmitUserInput(projectID, runID, "", userInput)
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

func (s *Service) HandleWatchSSE(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimPrefix(r.URL.Path, "/api/watch/")
	if runID == "" {
		http.Error(w, "run_id required", http.StatusBadRequest)
		return
	}
	eventCh, ok := s.app.RunEventChannel(runID)
	if !ok {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-eventCh:
			if !ok {
				fmt.Fprintf(w, "event: close\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			data, err := json.Marshal(map[string]any{
				"eventType":       event.EventType.String(),
				"message":         event.Message,
				"progressPercent": event.ProgressPercent,
				"clientView":      event.ClientView,
			})
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			if event.EventType == insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE || event.EventType == insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// worker launch wiring
// ---------------------------------------------------------------------------

func (s *Service) launchWorkerRun(projectID, workerKey, userInput string) (string, error) {
	return runuc.LaunchWorkerRun(projectID, workerKey, userInput, runuc.LaunchWorkerDeps{
		GetSession: func(projectID string) (runuc.WorkerSession, bool) {
			sess, ok := s.getProjectState(projectID)
			if !ok || sess.RunCtx == nil {
				return runuc.WorkerSession{}, false
			}
			return runuc.WorkerSession{ProjectID: sess.ProjectID, Env: sess.RunCtx.Env}, true
		},
		NewRunID:        func(workerKey string) string { return fmt.Sprintf("%s-%d", workerKey, time.Now().UnixNano()) },
		MarkRunStarted:  s.app.MarkRunStarted,
		MarkRunFinished: s.app.MarkRunFinished,
		AllocateEventCh: func(runID string) chan *insightifyv1.WatchRunResponse {
			return s.app.AllocateRunEventChannel(runID, 128)
		},
		CloseEventCh:       func(ch chan *insightifyv1.WatchRunResponse) { close(ch) },
		ClearNeedInput:     s.app.ClearUserInput,
		ClearRunNode:       s.app.ClearRunNode,
		ScheduleRunCleanup: s.app.ScheduleRunCleanup,
		ExecuteWorkerRun: func(sess runuc.WorkerSession, runID, workerKey, userInput string, eventCh chan *insightifyv1.WatchRunResponse) {
			runuc.ExecuteWorkerRun(sess, runID, workerKey, userInput, eventCh, runuc.ExecuteWorkerDeps{
				RegisterPendingUserInput: s.app.RegisterNeedInput,
				WaitPendingUserInput:     s.app.WaitUserInput,
				EmitRunEvent:             s.emitRunEvent,
				SetRunNode:               s.app.SetRunNode,
				ClearRunNode:             s.app.ClearRunNode,
				ToProtoUINode:            toProtoUINode,
			})
		},
	})
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
