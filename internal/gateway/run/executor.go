package run

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	pipelinev1 "insightify/gen/go/pipeline/v1"
	"insightify/internal/runner"
	internalui "insightify/internal/ui"
	"insightify/internal/workers/plan"
)

// WorkerSession holds the per-run worker execution state.
type WorkerSession struct {
	ProjectID string
	Env       *runner.Env
}

// executeWorkerRun runs a worker pipeline to completion, emitting events.
func (s *Service) executeWorkerRun(sess WorkerSession, runID, workerKey, userInput string, eventCh chan<- *insightifyv1.WatchRunResponse) {
	log.Printf("[run] executeWorkerRun begin: project_id=%s run_id=%s worker=%s initial_input_len=%d", sess.ProjectID, runID, workerKey, len(strings.TrimSpace(userInput)))
	s.trace(runID, "executor", "begin", map[string]any{
		"project_id":        sess.ProjectID,
		"worker":            workerKey,
		"initial_input_len": len(strings.TrimSpace(userInput)),
	})
	if sess.Env == nil || sess.Env.Resolver == nil {
		log.Printf("[run] executeWorkerRun invalid env: run_id=%s worker=%s", runID, workerKey)
		s.trace(runID, "executor", "invalid_env", map[string]any{"worker": workerKey})
		s.emit(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
			Message:   fmt.Sprintf("%s: run context is not configured", workerKey),
		})
		return
	}
	spec, ok := sess.Env.Resolver.Get(workerKey)
	if !ok {
		log.Printf("[run] executeWorkerRun unknown worker: run_id=%s worker=%s", runID, workerKey)
		s.trace(runID, "executor", "unknown_worker", map[string]any{"worker": workerKey})
		s.emit(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
			Message:   fmt.Sprintf("%s worker is not registered", workerKey),
		})
		return
	}

	nextInput := strings.TrimSpace(userInput)
	for {
		internalCh := make(chan runner.RunEvent, 100)
		emitter := &runner.ChannelEmitter{Ch: internalCh, Worker: workerKey}
		go s.bridgeRunnerEvents(sess.ProjectID, runID, eventCh, internalCh)
		uiEmitter := &runUIEmitter{
			svc:       s,
			projectID: sess.ProjectID,
			runID:     runID,
			eventCh:   eventCh,
		}

		baseCtx := runner.WithUserInput(context.Background(), nextInput)
		execCtx := internalui.WithEmitter(runner.WithEmitter(baseCtx, emitter), uiEmitter)
		log.Printf("[run] executeWorkerRun call worker: run_id=%s worker=%s input_len=%d", runID, workerKey, len(strings.TrimSpace(nextInput)))
		s.trace(runID, "executor", "call_worker", map[string]any{
			"worker":    workerKey,
			"input_len": len(strings.TrimSpace(nextInput)),
		})
		out, err := runner.ExecuteWorkerWithResult(execCtx, spec, sess.Env)
		close(internalCh)
		if err != nil {
			log.Printf("[run] executeWorkerRun worker error: run_id=%s worker=%s err=%v", runID, workerKey, err)
			s.trace(runID, "executor", "worker_error", map[string]any{
				"worker": workerKey,
				"error":  err.Error(),
			})
			s.emit(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
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
			requestID, err := s.interaction.RegisterNeedInput(sess.ProjectID, runID, workerKey, prompt)
			if err != nil {
				log.Printf("[run] executeWorkerRun register need input failed: run_id=%s worker=%s err=%v", runID, workerKey, err)
				s.trace(runID, "executor", "register_need_input_failed", map[string]any{
					"worker": workerKey,
					"error":  err.Error(),
				})
				s.emit(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
					EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
					Message:   err.Error(),
				})
				return
			}
			s.emit(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
				EventType:      insightifyv1.WatchRunResponse_EVENT_TYPE_INPUT_REQUIRED,
				InputRequestId: requestID,
				ClientView:     finalView,
			})
			log.Printf("[run] executeWorkerRun waiting input: run_id=%s worker=%s input_request_id=%s prompt_len=%d", runID, workerKey, requestID, len(prompt))
			s.trace(runID, "executor", "waiting_input", map[string]any{
				"worker":           workerKey,
				"input_request_id": requestID,
				"prompt_len":       len(prompt),
			})

			reply, err := s.interaction.WaitUserInput(runID, 10*time.Minute)
			if err != nil {
				log.Printf("[run] executeWorkerRun wait input error: run_id=%s worker=%s err=%v", runID, workerKey, err)
				s.trace(runID, "executor", "wait_input_error", map[string]any{
					"worker": workerKey,
					"error":  err.Error(),
				})
				s.emit(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
					EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
					Message:   err.Error(),
				})
				return
			}
			log.Printf("[run] executeWorkerRun got input: run_id=%s worker=%s reply_len=%d", runID, workerKey, len(strings.TrimSpace(reply)))
			s.trace(runID, "executor", "got_input", map[string]any{
				"worker":    workerKey,
				"reply_len": len(strings.TrimSpace(reply)),
			})
			nextInput = reply
			continue
		}

		log.Printf("[run] executeWorkerRun complete: run_id=%s worker=%s", runID, workerKey)
		s.trace(runID, "executor", "complete", map[string]any{"worker": workerKey})
		s.emit(sess.ProjectID, runID, eventCh, &insightifyv1.WatchRunResponse{
			EventType:  insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE,
			Message:    "COMPLETE",
			ClientView: finalView,
		})
		return
	}
}

// ---------------------------------------------------------------------------
// event helpers
// ---------------------------------------------------------------------------

type runUIEmitter struct {
	svc       *Service
	projectID string
	runID     string
	eventCh   chan<- *insightifyv1.WatchRunResponse
}

func (e *runUIEmitter) EmitUIEvent(event internalui.Event) {
	if e == nil {
		return
	}
	switch event.Type {
	case internalui.EventTypeUpsertNode:
		node := toProtoUINode(event.Node)
		if node == nil {
			return
		}
		log.Printf("[run] ui node upsert: run_id=%s node_id=%s type=%s", e.runID, node.GetId(), node.GetType().String())
		e.svc.trace(e.runID, "ui", "upsert_node", map[string]any{
			"node_id":   node.GetId(),
			"node_type": node.GetType().String(),
		})
		e.svc.uiStore.Set(e.runID, node)
		e.svc.emit(e.projectID, e.runID, e.eventCh, &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_NODE_READY,
			Node:      node,
		})
	case internalui.EventTypeRemoveNode:
		log.Printf("[run] ui node clear: run_id=%s", e.runID)
		e.svc.trace(e.runID, "ui", "clear_node", nil)
		e.svc.uiStore.Clear(e.runID)
	}
}

func (s *Service) emit(projectID, runID string, eventCh chan<- *insightifyv1.WatchRunResponse, ev *insightifyv1.WatchRunResponse) {
	if ev == nil || eventCh == nil {
		return
	}
	eventCh <- ev
}

func (s *Service) bridgeRunnerEvents(projectID, runID string, eventCh chan<- *insightifyv1.WatchRunResponse, internalCh <-chan runner.RunEvent) {
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
		s.emit(projectID, runID, eventCh, protoEvent)
	}
}

func extractWorkerClientView(clientView any) *pipelinev1.ClientView {
	if v, ok := clientView.(*pipelinev1.ClientView); ok && v != nil {
		return v
	}
	return nil
}
