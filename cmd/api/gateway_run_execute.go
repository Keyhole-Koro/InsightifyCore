package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	pipelinev1 "insightify/gen/go/pipeline/v1"
	"insightify/internal/runner"
	"insightify/internal/ui"
	"insightify/internal/workers/plan"
)

// launchWorkerRun launches a worker execution for the given project.
// This is a generic handler that can run any registered worker.
func (s *apiServer) launchWorkerRun(projectID, workerKey, userInput string) (string, error) {
	sess, ok := getProjectState(projectID)
	if !ok || sess.RunCtx == nil {
		return "", fmt.Errorf("project %s not found", projectID)
	}
	runID := fmt.Sprintf("%s-%d", workerKey, time.Now().UnixNano())
	sess, ok = updateProjectState(projectID, func(cur *projectState) {
		cur.Running = true
		cur.ActiveRunID = runID
	})
	if !ok {
		return "", fmt.Errorf("project %s not found", projectID)
	}

	eventCh := make(chan *insightifyv1.WatchRunResponse, 128)
	runStore.Lock()
	runStore.runs[runID] = eventCh
	runStore.Unlock()

	go func() {
		defer func() {
			clearPendingUserInput(runID)
			clearRunNode(runID)
			_, _ = updateProjectState(projectID, func(current *projectState) {
				if current.ActiveRunID == runID {
					current.ActiveRunID = ""
				}
				// Running reflects whether this project still has a designated active run.
				current.Running = strings.TrimSpace(current.ActiveRunID) != ""
			})
			close(eventCh)
			scheduleRunCleanup(runID)
		}()

		s.executeWorkerRun(sess, runID, workerKey, userInput, eventCh)
	}()

	return runID, nil
}

func (s *apiServer) executeWorkerRun(sess projectState, runID, workerKey, userInput string, eventCh chan<- *insightifyv1.WatchRunResponse) {
	if sess.RunCtx == nil || sess.RunCtx.Env == nil || sess.RunCtx.Env.Resolver == nil {
		emitRunEvent(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
			Message:   fmt.Sprintf("%s: run context is not configured", workerKey),
		})
		return
	}
	runCtx := sess.RunCtx
	spec, ok := runCtx.Env.Resolver.Get(workerKey)
	if !ok {
		emitRunEvent(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
			Message:   fmt.Sprintf("%s worker is not registered", workerKey),
		})
		return
	}

	nextInput := strings.TrimSpace(userInput)

	for {
		internalCh := make(chan runner.RunEvent, 100)
		emitter := &runner.ChannelEmitter{Ch: internalCh, Worker: workerKey}
		go bridgeRunnerEvents(sess.ProjectID, runID, eventCh, internalCh)
		uiEmitter := &runUIEmitter{
			projectID: sess.ProjectID,
			runID:     runID,
			eventCh:   eventCh,
		}

		baseCtx := runner.WithUserInput(context.Background(), nextInput)
		execCtx := ui.WithEmitter(runner.WithEmitter(baseCtx, emitter), uiEmitter)
		out, err := runner.ExecuteWorkerWithResult(execCtx, spec, runCtx.Env)
		close(internalCh)
		if err != nil {
			emitRunEvent(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
				EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
				Message:   err.Error(),
			})
			return
		}

		finalView := extractWorkerClientView(out.ClientView)
		if outBootstrap, ok := out.RuntimeState.(plan.BootstrapOut); ok && outBootstrap.NeedMoreInput() {
			prompt := strings.TrimSpace(outBootstrap.Result.FollowupQuestion)
			if prompt == "" {
				prompt = strings.TrimSpace(outBootstrap.Result.AssistantMessage)
			}
			requestID, err := registerPendingUserInput(sess.ProjectID, runID, workerKey, prompt)
			if err != nil {
				emitRunEvent(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
					EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
					Message:   err.Error(),
				})
				return
			}
			emitRunEvent(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
				EventType:  insightifyv1.WatchRunResponse_EVENT_TYPE_PROGRESS,
				Message:    fmt.Sprintf("INPUT_REQUIRED:%s", requestID),
				ClientView: finalView,
			})

			reply, err := waitPendingUserInput(runID, 10*time.Minute)
			if err != nil {
				emitRunEvent(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
					EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
					Message:   err.Error(),
				})
				return
			}
			nextInput = reply
			continue
		}

		emitRunEvent(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
			EventType:  insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE,
			Message:    "COMPLETE",
			ClientView: finalView,
		})
		return
	}
}

type runUIEmitter struct {
	projectID string
	runID     string
	eventCh   chan<- *insightifyv1.WatchRunResponse
}

func (e *runUIEmitter) EmitUIEvent(event ui.Event) {
	if e == nil {
		return
	}
	switch event.Type {
	case ui.EventTypeUpsertNode:
		node := toProtoUINode(event.Node)
		if node == nil {
			return
		}
		setRunNode(e.runID, node)
		if e.eventCh != nil {
			emitRunEvent(e.projectID, e.runID, e.eventCh, &insightifyv1.WatchRunResponse{
				EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_PROGRESS,
				Message:   nodeReadyPrefix,
			})
		}
	case ui.EventTypeRemoveNode:
		clearRunNode(e.runID)
	}
}

// bridgeRunnerEvents converts runner events to proto events.
func bridgeRunnerEvents(projectID, runID string, eventCh chan<- *insightifyv1.WatchRunResponse, internalCh <-chan runner.RunEvent) {
	for ev := range internalCh {
		protoEvent := &insightifyv1.WatchRunResponse{
			Message:         ev.Message,
			ProgressPercent: ev.Progress,
		}
		switch ev.Type {
		case runner.EventTypeLog:
			protoEvent.EventType = insightifyv1.WatchRunResponse_EVENT_TYPE_LOG
		case runner.EventTypeProgress:
			protoEvent.EventType = insightifyv1.WatchRunResponse_EVENT_TYPE_PROGRESS
		case runner.EventTypeLLMChunk:
			protoEvent.EventType = insightifyv1.WatchRunResponse_EVENT_TYPE_LOG
			protoEvent.Message = ev.Chunk
		default:
			continue
		}
		emitRunEvent(projectID, runID, eventCh, protoEvent)
	}
}

func emitRunEvent(projectID, runID string, eventCh chan<- *insightifyv1.WatchRunResponse, ev *insightifyv1.WatchRunResponse) {
	if ev == nil {
		return
	}
	if eventCh != nil {
		eventCh <- ev
	}
	publishRunEventToChat(projectID, runID, ev)
}

// extractWorkerClientView extracts the ClientView from worker output.
func extractWorkerClientView(clientView any) *pipelinev1.ClientView {
	if v, ok := clientView.(*pipelinev1.ClientView); ok && v != nil {
		return v
	}
	return nil
}

// launchBootstrapRun is a convenience wrapper for launching the bootstrap worker.
func (s *apiServer) launchBootstrapRun(projectID, userInput string) (string, error) {
	return s.launchWorkerRun(projectID, "bootstrap", userInput)
}
