package run

import (
	"context"
	"fmt"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	pipelinev1 "insightify/gen/go/pipeline/v1"
	chatuc "insightify/internal/gateway/usecase/chat"
	"insightify/internal/runner"
	"insightify/internal/ui"
	"insightify/internal/workers/plan"
)

type WorkerSession struct {
	ProjectID string
	Env       *runner.Env
}

type LaunchWorkerDeps struct {
	GetSession         func(projectID string) (WorkerSession, bool)
	NewRunID           func(workerKey string) string
	MarkRunStarted     func(projectID, runID string)
	MarkRunFinished    func(projectID, runID string)
	AllocateEventCh    func(runID string) chan *insightifyv1.WatchRunResponse
	CloseEventCh       func(ch chan *insightifyv1.WatchRunResponse)
	ClearNeedInput     func(runID string)
	ClearRunNode       func(runID string)
	ScheduleRunCleanup func(runID string)
	ExecuteWorkerRun   func(sess WorkerSession, runID, workerKey, userInput string, eventCh chan *insightifyv1.WatchRunResponse)
}

func LaunchWorkerRun(projectID, workerKey, userInput string, deps LaunchWorkerDeps) (string, error) {
	if deps.GetSession == nil {
		return "", fmt.Errorf("run session resolver is not configured")
	}
	sess, ok := deps.GetSession(projectID)
	if !ok || sess.Env == nil || sess.Env.Resolver == nil {
		return "", fmt.Errorf("project %s not found", projectID)
	}
	if deps.NewRunID == nil {
		return "", fmt.Errorf("run id generator is not configured")
	}
	runID := deps.NewRunID(workerKey)
	if deps.MarkRunStarted != nil {
		deps.MarkRunStarted(projectID, runID)
	}

	var eventCh chan *insightifyv1.WatchRunResponse
	if deps.AllocateEventCh != nil {
		eventCh = deps.AllocateEventCh(runID)
	}
	if eventCh == nil {
		eventCh = make(chan *insightifyv1.WatchRunResponse, 128)
	}

	go func() {
		defer func() {
			if deps.ClearNeedInput != nil {
				deps.ClearNeedInput(runID)
			}
			if deps.ClearRunNode != nil {
				deps.ClearRunNode(runID)
			}
			if deps.MarkRunFinished != nil {
				deps.MarkRunFinished(projectID, runID)
			}
			if deps.CloseEventCh != nil {
				deps.CloseEventCh(eventCh)
			} else {
				close(eventCh)
			}
			if deps.ScheduleRunCleanup != nil {
				deps.ScheduleRunCleanup(runID)
			}
		}()
		if deps.ExecuteWorkerRun != nil {
			deps.ExecuteWorkerRun(sess, runID, workerKey, userInput, eventCh)
		}
	}()

	return runID, nil
}

type ExecuteWorkerDeps struct {
	RegisterPendingUserInput func(projectID, runID, workerKey, prompt string) (string, error)
	WaitPendingUserInput     func(runID string, timeout time.Duration) (string, error)
	EmitRunEvent             func(projectID, runID string, eventCh chan<- *insightifyv1.WatchRunResponse, ev *insightifyv1.WatchRunResponse)
	SetRunNode               func(runID string, node *insightifyv1.UiNode)
	ClearRunNode             func(runID string)
	ToProtoUINode            func(node ui.Node) *insightifyv1.UiNode
}

func ExecuteWorkerRun(sess WorkerSession, runID, workerKey, userInput string, eventCh chan<- *insightifyv1.WatchRunResponse, deps ExecuteWorkerDeps) {
	if sess.Env == nil || sess.Env.Resolver == nil {
		emit(deps, sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
			Message:   fmt.Sprintf("%s: run context is not configured", workerKey),
		})
		return
	}
	spec, ok := sess.Env.Resolver.Get(workerKey)
	if !ok {
		emit(deps, sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
			Message:   fmt.Sprintf("%s worker is not registered", workerKey),
		})
		return
	}

	nextInput := strings.TrimSpace(userInput)
	for {
		internalCh := make(chan runner.RunEvent, 100)
		emitter := &runner.ChannelEmitter{Ch: internalCh, Worker: workerKey}
		go bridgeRunnerEvents(sess.ProjectID, runID, eventCh, internalCh, deps)
		uiEmitter := &runUIEmitter{
			projectID: sess.ProjectID,
			runID:     runID,
			eventCh:   eventCh,
			deps:      deps,
		}

		baseCtx := runner.WithUserInput(context.Background(), nextInput)
		execCtx := ui.WithEmitter(runner.WithEmitter(baseCtx, emitter), uiEmitter)
		out, err := runner.ExecuteWorkerWithResult(execCtx, spec, sess.Env)
		close(internalCh)
		if err != nil {
			emit(deps, sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
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
			requestID, err := deps.RegisterPendingUserInput(sess.ProjectID, runID, workerKey, prompt)
			if err != nil {
				emit(deps, sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
					EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
					Message:   err.Error(),
				})
				return
			}
			emit(deps, sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
				EventType:  insightifyv1.WatchRunResponse_EVENT_TYPE_PROGRESS,
				Message:    fmt.Sprintf("%s%s", chatuc.InputRequiredPrefix, requestID),
				ClientView: finalView,
			})

			reply, err := deps.WaitPendingUserInput(runID, 10*time.Minute)
			if err != nil {
				emit(deps, sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
					EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
					Message:   err.Error(),
				})
				return
			}
			nextInput = reply
			continue
		}

		emit(deps, sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
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
	deps      ExecuteWorkerDeps
}

func (e *runUIEmitter) EmitUIEvent(event ui.Event) {
	if e == nil {
		return
	}
	switch event.Type {
	case ui.EventTypeUpsertNode:
		if e.deps.ToProtoUINode == nil {
			return
		}
		node := e.deps.ToProtoUINode(event.Node)
		if node == nil {
			return
		}
		if e.deps.SetRunNode != nil {
			e.deps.SetRunNode(e.runID, node)
		}
		emit(e.deps, e.projectID, e.runID, e.eventCh, &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_PROGRESS,
			Message:   chatuc.NodeReadyPrefix,
		})
	case ui.EventTypeRemoveNode:
		if e.deps.ClearRunNode != nil {
			e.deps.ClearRunNode(e.runID)
		}
	}
}

func bridgeRunnerEvents(projectID, runID string, eventCh chan<- *insightifyv1.WatchRunResponse, internalCh <-chan runner.RunEvent, deps ExecuteWorkerDeps) {
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
		emit(deps, projectID, runID, eventCh, protoEvent)
	}
}

func emit(deps ExecuteWorkerDeps, projectID, runID string, eventCh chan<- *insightifyv1.WatchRunResponse, ev *insightifyv1.WatchRunResponse) {
	if ev == nil {
		return
	}
	if deps.EmitRunEvent != nil {
		deps.EmitRunEvent(projectID, runID, eventCh, ev)
		return
	}
	if eventCh != nil {
		eventCh <- ev
	}
}

func extractWorkerClientView(clientView any) *pipelinev1.ClientView {
	if v, ok := clientView.(*pipelinev1.ClientView); ok && v != nil {
		return v
	}
	return nil
}
