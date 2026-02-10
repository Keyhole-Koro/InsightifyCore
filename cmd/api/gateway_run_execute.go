package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	pipelinev1 "insightify/gen/go/pipeline/v1"
	"insightify/internal/runner"
	"insightify/internal/workers/plan"
)

// launchWorkerRun launches a worker execution for the given session.
// This is a generic handler that can run any registered worker.
func (s *apiServer) launchWorkerRun(sessionID, workerKey, userInput string, isBootstrap bool) (string, error) {
	sess, ok := getSession(sessionID)
	if !ok || sess.RunCtx == nil {
		return "", fmt.Errorf("session %s not found", sessionID)
	}
	if sess.Running {
		return "", fmt.Errorf("session %s already has an active run", sessionID)
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
			_, _ = updateSession(sessionID, func(current *initSession) {
				current.Running = false
				if current.ActiveRunID == runID {
					current.ActiveRunID = ""
				}
			})
			close(eventCh)
			scheduleRunCleanup(runID)
		}()

		s.executeWorkerRun(sess, workerKey, userInput, isBootstrap, eventCh)
	}()

	return runID, nil
}

func (s *apiServer) executeWorkerRun(sess initSession, workerKey, userInput string, isBootstrap bool, eventCh chan<- *insightifyv1.WatchRunResponse) {
	if sess.RunCtx == nil || sess.RunCtx.Env == nil || sess.RunCtx.Env.Resolver == nil {
		eventCh <- &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
			Message:   fmt.Sprintf("%s: run context is not configured", workerKey),
		}
		return
	}
	runCtx := sess.RunCtx
	runCtx.Env.InitPurposeUserInput = strings.TrimSpace(userInput)
	runCtx.Env.InitPurposeBootstrap = isBootstrap

	spec, ok := runCtx.Env.Resolver.Get(workerKey)
	if !ok {
		eventCh <- &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
			Message:   fmt.Sprintf("%s worker is not registered", workerKey),
		}
		return
	}

	internalCh := make(chan runner.RunEvent, 100)
	emitter := &runner.ChannelEmitter{Ch: internalCh, Worker: workerKey}
	go bridgeRunnerEvents(eventCh, internalCh)

	execCtx := runner.WithEmitter(context.Background(), emitter)
	out, err := runner.ExecuteWorkerWithResult(execCtx, spec, runCtx.Env)
	close(internalCh)
	if err != nil {
		eventCh <- &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
			Message:   err.Error(),
		}
		return
	}

	// Update session from result (worker-specific logic)
	updateSessionFromResult(sess.SessionID, runCtx, out.RuntimeState)

	// Build final response
	finalView := extractWorkerClientView(out.ClientView, out.RuntimeState)
	finalMessage := determineCompletionMessage(out.RuntimeState)

	eventCh <- &insightifyv1.WatchRunResponse{
		EventType:  insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE,
		Message:    finalMessage,
		ClientView: finalView,
	}
}

// bridgeRunnerEvents converts runner events to proto events.
func bridgeRunnerEvents(eventCh chan<- *insightifyv1.WatchRunResponse, internalCh <-chan runner.RunEvent) {
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
		eventCh <- protoEvent
	}
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

// determineCompletionMessage determines the completion message based on runtime state.
func determineCompletionMessage(runtimeState any) string {
	// Check if more input is needed (for interactive workers)
	if out, ok := runtimeState.(plan.BootstrapOut); ok {
		if out.NeedMoreInput() {
			return "INPUT_REQUIRED"
		}
	}
	return "COMPLETE"
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
				if repo := inferRepoName(cur.RepoURL); repo != "" {
					cur.Repo = repo
				}
			}
			if runCtx != nil && runCtx.Env != nil {
				if trimmedPurpose != "" {
					runCtx.Env.InitPurpose = trimmedPurpose
				}
				if trimmedRepoURL != "" {
					runCtx.Env.InitPurposeRepoURL = trimmedRepoURL
				}
			}
			cur.RunCtx = runCtx
		})

		persistSessionStore()
	}
}

// launchPlanPipelineRun is a convenience wrapper for launching the plan_pipeline worker.
func (s *apiServer) launchPlanPipelineRun(sessionID, userInput string, isBootstrap bool) (string, error) {
	return s.launchWorkerRun(sessionID, "plan_pipeline", userInput, isBootstrap)
}
