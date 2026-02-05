package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/gen/go/insightify/v1/insightifyv1connect"
	pipelinev1 "insightify/gen/go/pipeline/v1"
	"insightify/internal/pipeline/testpipe"
	"insightify/internal/runner"

	"connectrpc.com/connect"
)

// runStore holds active runs and their event channels.
var runStore = struct {
	sync.RWMutex
	runs map[string]chan *insightifyv1.RunEvent
}{
	runs: make(map[string]chan *insightifyv1.RunEvent),
}

// StartRun executes a single pipeline phase and returns the result.
func (s *apiServer) StartRun(ctx context.Context, req *connect.Request[insightifyv1.StartRunRequest]) (*connect.Response[insightifyv1.StartRunResponse], error) {
	pipelineID := req.Msg.GetPipelineId()

	// Handle test_pipeline separately for streaming demo
	if pipelineID == "test_pipeline" {
		runID := fmt.Sprintf("test-%d", time.Now().UnixNano())

		// Create event channel for this run
		eventCh := make(chan *insightifyv1.RunEvent, 20)
		runStore.Lock()
		runStore.runs[runID] = eventCh
		runStore.Unlock()

		// Start pipeline in background
		go func() {
			defer func() {
				runStore.Lock()
				delete(runStore.runs, runID)
				runStore.Unlock()
				close(eventCh)
			}()

			pipeline := &testpipe.TestStreamingPipeline{}
			progressCh := make(chan testpipe.StreamStep, 10)

			go func() {
				for step := range progressCh {
					eventCh <- &insightifyv1.RunEvent{
						EventType:       insightifyv1.RunEvent_EVENT_TYPE_LOG,
						Message:         step.Message,
						ProgressPercent: step.Progress,
						ClientView:      step.View,
					}
				}
			}()

			result, err := pipeline.Run(context.Background(), progressCh)
			if err != nil {
				eventCh <- &insightifyv1.RunEvent{
					EventType: insightifyv1.RunEvent_EVENT_TYPE_ERROR,
					Message:   err.Error(),
				}
				return
			}

			eventCh <- &insightifyv1.RunEvent{
				EventType:  insightifyv1.RunEvent_EVENT_TYPE_COMPLETE,
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
	eventCh := make(chan *insightifyv1.RunEvent, 100)
	runStore.Lock()
	runStore.runs[runID] = eventCh
	runStore.Unlock()

	// Original logic for other pipelines
	repoName := req.Msg.GetParams()["repo_name"]
	if repoName == "" {
		repoName = "PoliTopics"
	}

	runCtx, err := NewRunContext(repoName)
	if err != nil {
		runStore.Lock()
		delete(runStore.runs, runID)
		runStore.Unlock()
		close(eventCh)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
	}

	key := pipelineID
	if key == "" {
		key = "phase_DAG"
	}

	spec, ok := runCtx.Env.Resolver.Get(key)
	if !ok {
		runCtx.Cleanup()
		runStore.Lock()
		delete(runStore.runs, runID)
		runStore.Unlock()
		close(eventCh)
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown phase %s", key))
	}

	// Start pipeline in background with emitter
	go func() {
		defer func() {
			runCtx.Cleanup()
			runStore.Lock()
			delete(runStore.runs, runID)
			runStore.Unlock()
			close(eventCh)
		}()

		// Create emitter that bridges runner events to proto events
		internalCh := make(chan runner.RunEvent, 100)
		emitter := &runner.ChannelEmitter{Ch: internalCh, Phase: key}

		// Bridge internal events to proto events
		go func() {
			for ev := range internalCh {
				protoEvent := &insightifyv1.RunEvent{
					Message:         ev.Message,
					ProgressPercent: ev.Progress,
				}
				switch ev.Type {
				case runner.EventTypeLog:
					protoEvent.EventType = insightifyv1.RunEvent_EVENT_TYPE_LOG
				case runner.EventTypeProgress:
					protoEvent.EventType = insightifyv1.RunEvent_EVENT_TYPE_PROGRESS
				case runner.EventTypeLLMChunk:
					protoEvent.EventType = insightifyv1.RunEvent_EVENT_TYPE_LOG
					protoEvent.Message = ev.Chunk
				default:
					continue
				}
				eventCh <- protoEvent
			}
		}()

		execCtx := runner.WithEmitter(context.Background(), emitter)
		out, err := runner.ExecutePhaseWithResult(execCtx, spec, runCtx.Env)
		close(internalCh)

		if err != nil {
			eventCh <- &insightifyv1.RunEvent{
				EventType: insightifyv1.RunEvent_EVENT_TYPE_ERROR,
				Message:   err.Error(),
			}
			return
		}

		finalEvent := &insightifyv1.RunEvent{
			EventType: insightifyv1.RunEvent_EVENT_TYPE_COMPLETE,
			Message:   "Done",
		}
		if out.ClientView != nil {
			if view, ok := out.ClientView.(*pipelinev1.ClientView); ok {
				finalEvent.ClientView = view
			} else if view, ok := out.ClientView.(pipelinev1.ClientView); ok {
				finalEvent.ClientView = &view
			}
		}
		eventCh <- finalEvent
	}()

	return connect.NewResponse(&insightifyv1.StartRunResponse{
		RunId: runID,
	}), nil
}

// WatchRun streams events for a running pipeline.
func (s *apiServer) WatchRun(ctx context.Context, req *connect.Request[insightifyv1.WatchRunRequest], stream *connect.ServerStream[insightifyv1.RunEvent]) error {
	runID := req.Msg.GetRunId()

	runStore.RLock()
	eventCh, ok := runStore.runs[runID]
	runStore.RUnlock()

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
				return nil // Channel closed, run complete
			}
			if err := stream.Send(event); err != nil {
				return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send event: %w", err))
			}
			if event.EventType == insightifyv1.RunEvent_EVENT_TYPE_COMPLETE ||
				event.EventType == insightifyv1.RunEvent_EVENT_TYPE_ERROR {
				return nil
			}
		}
	}
}

// handleWatchSSE handles Server-Sent Events for watching a run.
func (s *apiServer) handleWatchSSE(w http.ResponseWriter, r *http.Request) {
	// Extract run_id from path: /api/watch/{run_id}
	runID := strings.TrimPrefix(r.URL.Path, "/api/watch/")
	if runID == "" {
		http.Error(w, "run_id required", http.StatusBadRequest)
		return
	}

	runStore.RLock()
	eventCh, ok := runStore.runs[runID]
	runStore.RUnlock()

	if !ok {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}

	// Set SSE headers
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
				// Channel closed, send final message
				fmt.Fprintf(w, "event: close\ndata: {}\n\n")
				flusher.Flush()
				return
			}

			// Convert to JSON
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

			// Close on terminal events
			if event.EventType == insightifyv1.RunEvent_EVENT_TYPE_COMPLETE ||
				event.EventType == insightifyv1.RunEvent_EVENT_TYPE_ERROR {
				return
			}
		}
	}
}

// Ensure interface conformance
var _ insightifyv1connect.PipelineServiceHandler = (*apiServer)(nil)
