package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	pipelinev1 "insightify/gen/go/pipeline/v1"
	"insightify/internal/runner"
	"insightify/internal/utils"
	"insightify/internal/workers/testpipe"

	"connectrpc.com/connect"
)

// StartRun executes a single pipeline worker and returns the result.
func (s *apiServer) StartRun(ctx context.Context, req *connect.Request[insightifyv1.StartRunRequest]) (*connect.Response[insightifyv1.StartRunResponse], error) {
	ensureSessionStoreLoaded()
	pipelineID := req.Msg.GetPipelineId()
	sessionID := resolveSessionID(req)
	if sessionID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required (request field or cookie)"))
	}
	sess, ok := getSession(sessionID)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session %s not found", sessionID))
	}
	var err error
	sess, err = ensureSessionRunContext(sessionID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// Handle plan_pipeline (and legacy init_purpose) via launchPlanPipelineRun
	if pipelineID == "plan_pipeline" || pipelineID == "init_purpose" {
		runID, err := s.launchPlanPipelineRun(sessionID, strings.TrimSpace(req.Msg.GetParams()["user_input"]), false)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to start plan_pipeline: %w", err))
		}
		return connect.NewResponse(&insightifyv1.StartRunResponse{
			RunId: runID,
		}), nil
	}
	if sess.Running {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session %s already has an active run", sessionID))
	}
	sess, ok = updateSession(sessionID, func(cur *initSession) {
		cur.Running = true
	})
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session %s not found", sessionID))
	}
	runCtx := sess.RunCtx

	// Handle test_pipeline separately for streaming demo
	if pipelineID == "test_pipeline" {
		runID := fmt.Sprintf("test-%d", time.Now().UnixNano())
		uidGen := utils.NewUIDGenerator()

		// Create event channel for this run
		eventCh := make(chan *insightifyv1.WatchRunResponse, 20)
		runStore.Lock()
		runStore.runs[runID] = eventCh
		runStore.Unlock()

		// Start pipeline in background
		go func() {
			defer func() {
				_, _ = updateSession(sessionID, func(cur *initSession) {
					cur.Running = false
				})
				close(eventCh)
				scheduleRunCleanup(runID)
			}()

			pipeline := &testpipe.TestStreamingPipeline{}
			progressCh := make(chan testpipe.StreamStep, 10)

			go func() {
				for step := range progressCh {
					utils.AssignGraphNodeUIDsWithGenerator(uidGen, step.View)
					eventCh <- &insightifyv1.WatchRunResponse{
						EventType:       insightifyv1.WatchRunResponse_EVENT_TYPE_LOG,
						Message:         step.Message,
						ProgressPercent: int32(step.Progress),
						ClientView:      step.View,
					}
				}
			}()

			result, err := pipeline.Run(context.Background(), progressCh)
			if err != nil {
				eventCh <- &insightifyv1.WatchRunResponse{
					EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR,
					Message:   err.Error(),
				}
				return
			}
			utils.AssignGraphNodeUIDsWithGenerator(uidGen, result)

			eventCh <- &insightifyv1.WatchRunResponse{
				EventType:  insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE,
				Message:    "Done",
				ClientView: result,
			}
		}()

		return connect.NewResponse(&insightifyv1.StartRunResponse{
			RunId: runID,
		}), nil
	}

	// Create run ID for streaming pipelines
	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())

	// Create event channel for this run
	eventCh := make(chan *insightifyv1.WatchRunResponse, 100)
	runStore.Lock()
	runStore.runs[runID] = eventCh
	runStore.Unlock()

	key := pipelineID
	if key == "" {
		key = "worker_DAG"
	}

	spec, ok := runCtx.Env.Resolver.Get(key)
	if !ok {
		_, _ = updateSession(sessionID, func(cur *initSession) {
			cur.Running = false
		})
		runStore.Lock()
		delete(runStore.runs, runID)
		runStore.Unlock()
		close(eventCh)
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown worker %s", key))
	}

	// Start pipeline in background with emitter
	go func() {
		defer func() {
			_, _ = updateSession(sessionID, func(cur *initSession) {
				cur.Running = false
			})
			close(eventCh)
			scheduleRunCleanup(runID)
		}()

		// Create emitter that bridges runner events to proto events
		internalCh := make(chan runner.RunEvent, 100)
		emitter := &runner.ChannelEmitter{Ch: internalCh, Worker: key}

		// Bridge internal events to proto events
		go func() {
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
		}()

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

		finalEvent := &insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE,
			Message:   "Done",
		}
		if out.ClientView != nil {
			if view, ok := out.ClientView.(*pipelinev1.ClientView); ok {
				finalEvent.ClientView = view
			}
		}
		eventCh <- finalEvent
	}()

	return connect.NewResponse(&insightifyv1.StartRunResponse{
		RunId: runID,
	}), nil
}
