package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"

	"connectrpc.com/connect"
)

// WatchRun streams events for a running pipeline.
func (s *apiServer) WatchRun(ctx context.Context, req *connect.Request[insightifyv1.WatchRunRequest], stream *connect.ServerStream[insightifyv1.WatchRunResponse]) error {
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
			if event.EventType == insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE ||
				event.EventType == insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR {
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
			if event.EventType == insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE ||
				event.EventType == insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR {
				return
			}
		}
	}
}
