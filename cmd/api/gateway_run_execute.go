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

// launchWorkerRun launches a worker execution for the given session.
// This is a generic handler that can run any registered worker.
func (s *apiServer) launchWorkerRun(sessionID, workerKey, userInput string) (string, error) {
	sess, ok := getSession(sessionID)
	if !ok || sess.RunCtx == nil {
		return "", fmt.Errorf("session %s not found", sessionID)
	}
	runID := fmt.Sprintf("%s-%d", workerKey, time.Now().UnixNano())
	sess, ok = updateSession(sessionID, func(cur *initSession) {
		cur.Running = true
		cur.ActiveRunID = runID
	})
	if !ok {
		return "", fmt.Errorf("session %s not found", sessionID)
	}

	eventCh := make(chan *insightifyv1.WatchRunResponse, 128)
	runStore.Lock()
	runStore.runs[runID] = eventCh
	runStore.Unlock()

	go func() {
		defer func() {
			clearPendingUserInput(runID)
			clearRunNode(runID)
			_, _ = updateSession(sessionID, func(current *initSession) {
				if current.ActiveRunID == runID {
					current.ActiveRunID = ""
				}
				// Running reflects whether this session still has a designated active run.
				current.Running = strings.TrimSpace(current.ActiveRunID) != ""
			})
			close(eventCh)
			scheduleRunCleanup(runID)
		}()

		s.executeWorkerRun(sess, runID, workerKey, userInput, eventCh)
	}()

	return runID, nil
}

func (s *apiServer) executeWorkerRun(sess initSession, runID, workerKey, userInput string, eventCh chan<- *insightifyv1.WatchRunResponse) {
	if sess.RunCtx == nil || sess.RunCtx.Env == nil || sess.RunCtx.Env.Resolver == nil {
		emitRunEvent(sess.SessionID, runID, eventCh, &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
			Message:   fmt.Sprintf("%s: run context is not configured", workerKey),
		})
		return
	}
	runCtx := sess.RunCtx
	spec, ok := runCtx.Env.Resolver.Get(workerKey)
	if !ok {
		emitRunEvent(sess.SessionID, runID, eventCh, &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
			Message:   fmt.Sprintf("%s worker is not registered", workerKey),
		})
		return
	}

	nextInput := strings.TrimSpace(userInput)

	for {
		runCtx.Env.InitCtx.UserInput = nextInput

		internalCh := make(chan runner.RunEvent, 100)
		emitter := &runner.ChannelEmitter{Ch: internalCh, Worker: workerKey}
		go bridgeRunnerEvents(sess.SessionID, runID, eventCh, internalCh)
		uiEmitter := &runUIEmitter{
			sessionID: sess.SessionID,
			runID:     runID,
			eventCh:   eventCh,
		}

		execCtx := ui.WithEmitter(runner.WithEmitter(context.Background(), emitter), uiEmitter)
		out, err := runner.ExecuteWorkerWithResult(execCtx, spec, runCtx.Env)
		close(internalCh)
		if err != nil {
			emitRunEvent(sess.SessionID, runID, eventCh, &insightifyv1.WatchRunResponse{
				EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
				Message:   err.Error(),
			})
			return
		}

		// Update session from result (worker-specific logic)
		updateSessionFromResult(sess.SessionID, runCtx, out.RuntimeState)

		finalView := extractWorkerClientView(out.ClientView, out.RuntimeState)
		if outBootstrap, ok := out.RuntimeState.(plan.BootstrapOut); ok && outBootstrap.NeedMoreInput() {
			setRunNode(runID, toProtoUINode(outBootstrap.UINode))
			prompt := strings.TrimSpace(outBootstrap.Result.FollowupQuestion)
			if prompt == "" {
				prompt = strings.TrimSpace(outBootstrap.Result.AssistantMessage)
			}
			requestID, err := registerPendingUserInput(sess.SessionID, runID, workerKey, prompt)
			if err != nil {
				emitRunEvent(sess.SessionID, runID, eventCh, &insightifyv1.WatchRunResponse{
					EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
					Message:   err.Error(),
				})
				return
			}
			emitRunEvent(sess.SessionID, runID, eventCh, &insightifyv1.WatchRunResponse{
				EventType:  insightifyv1.WatchRunResponse_EVENT_TYPE_PROGRESS,
				Message:    fmt.Sprintf("INPUT_REQUIRED:%s", requestID),
				ClientView: finalView,
			})

			reply, err := waitPendingUserInput(runID, 10*time.Minute)
			if err != nil {
				emitRunEvent(sess.SessionID, runID, eventCh, &insightifyv1.WatchRunResponse{
					EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
					Message:   err.Error(),
				})
				return
			}
			nextInput = reply
			continue
		}
		if outBootstrap, ok := out.RuntimeState.(plan.BootstrapOut); ok {
			setRunNode(runID, toProtoUINode(outBootstrap.UINode))
		}

		emitRunEvent(sess.SessionID, runID, eventCh, &insightifyv1.WatchRunResponse{
			EventType:  insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE,
			Message:    "COMPLETE",
			ClientView: finalView,
		})
		return
	}
}

type runUIEmitter struct {
	sessionID string
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
			emitRunEvent(e.sessionID, e.runID, e.eventCh, &insightifyv1.WatchRunResponse{
				EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_PROGRESS,
				Message:   nodeReadyPrefix,
			})
		}
	case ui.EventTypeRemoveNode:
		clearRunNode(e.runID)
	}
}

// bridgeRunnerEvents converts runner events to proto events.
func bridgeRunnerEvents(sessionID, runID string, eventCh chan<- *insightifyv1.WatchRunResponse, internalCh <-chan runner.RunEvent) {
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
		emitRunEvent(sessionID, runID, eventCh, protoEvent)
	}
}

func emitRunEvent(sessionID, runID string, eventCh chan<- *insightifyv1.WatchRunResponse, ev *insightifyv1.WatchRunResponse) {
	if ev == nil {
		return
	}
	if eventCh != nil {
		eventCh <- ev
	}
	publishRunEventToChat(sessionID, runID, ev)
}

// extractWorkerClientView extracts the ClientView from worker output.
func extractWorkerClientView(clientView any, runtimeState any) *pipelinev1.ClientView {
	if v, ok := clientView.(*pipelinev1.ClientView); ok && v != nil {
		return v
	}
	// Fallback for BootstrapOut
	if out, ok := runtimeState.(plan.BootstrapOut); ok {
		return out.ClientView
	}
	return nil
}

// updateSessionFromResult updates session state based on worker output.
func updateSessionFromResult(sessionID string, runCtx *RunContext, runtimeState any) {
	// Handle BootstrapOut
	if out, ok := runtimeState.(plan.BootstrapOut); ok {
		trimmedPurpose := strings.TrimSpace(out.Result.Purpose)
		trimmedRepoURL := strings.TrimSpace(out.Result.RepoURL)
		if trimmedPurpose == "" && trimmedRepoURL == "" {
			return
		}

		_, _ = updateSession(sessionID, func(cur *initSession) {
			if trimmedPurpose != "" {
				cur.Purpose = trimmedPurpose
			}
			if trimmedRepoURL != "" {
				cur.RepoURL = trimmedRepoURL
			}
			if runCtx != nil && runCtx.Env != nil {
				runCtx.Env.InitCtx.SetPurpose(trimmedPurpose, trimmedRepoURL)
			}
			cur.RunCtx = runCtx
		})

		persistSessionStore()
	}
}

// launchBootstrapRun is a convenience wrapper for launching the bootstrap worker.
func (s *apiServer) launchBootstrapRun(sessionID, userInput string) (string, error) {
	return s.launchWorkerRun(sessionID, "bootstrap", userInput)
}

// launchInitPurposeRun is kept as a compatibility alias.
func (s *apiServer) launchInitPurposeRun(sessionID, userInput string) (string, error) {
	return s.launchBootstrapRun(sessionID, userInput)
}
