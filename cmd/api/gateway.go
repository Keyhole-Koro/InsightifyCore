package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/gen/go/insightify/v1/insightifyv1connect"
	pipelinev1 "insightify/gen/go/pipeline/v1"
	"insightify/internal/pipeline/testpipe"
	"insightify/internal/runner"
	"insightify/internal/utils"

	"connectrpc.com/connect"
)

const sessionCookieName = "insightify_session_id"

// runStore holds active runs and their event channels.
var runStore = struct {
	sync.RWMutex
	runs map[string]chan *insightifyv1.WatchRunResponse
}{
	runs: make(map[string]chan *insightifyv1.WatchRunResponse),
}

type initSession struct {
	UserID  string
	RepoURL string
	Repo    string
	RunCtx  *RunContext
	Running bool
}

var initRunStore = struct {
	sync.RWMutex
	sessions map[string]initSession
}{
	sessions: make(map[string]initSession),
}

// InitRun initializes a run session. Current implementation is a lightweight mock.
func (s *apiServer) InitRun(_ context.Context, req *connect.Request[insightifyv1.InitRunRequest]) (*connect.Response[insightifyv1.InitRunResponse], error) {
	userID := strings.TrimSpace(req.Msg.GetUserId())
	repoURL := strings.TrimSpace(req.Msg.GetRepoUrl())
	if userID == "" || repoURL == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id and repo_url are required"))
	}

	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())
	repoName := inferRepoName(repoURL)
	if repoName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid repo_url: could not infer repository name"))
	}
	runCtx, err := NewRunContext(repoName, sessionID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
	}

	initRunStore.Lock()
	initRunStore.sessions[sessionID] = initSession{
		UserID:  userID,
		RepoURL: repoURL,
		Repo:    repoName,
		RunCtx:  runCtx,
		Running: false,
	}
	initRunStore.Unlock()

	res := connect.NewResponse(&insightifyv1.InitRunResponse{
		SessionId: sessionID,
		RepoName:  repoName,
	})
	res.Header().Add("Set-Cookie", (&http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Local development uses plain HTTP; enable Secure in TLS deployments.
		Secure: false,
	}).String())
	return res, nil
}

// StartRun executes a single pipeline worker and returns the result.
func (s *apiServer) StartRun(ctx context.Context, req *connect.Request[insightifyv1.StartRunRequest]) (*connect.Response[insightifyv1.StartRunResponse], error) {
	pipelineID := req.Msg.GetPipelineId()
	sessionID := resolveSessionID(req)
	if sessionID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required (request field or cookie)"))
	}
	initRunStore.Lock()
	sess, ok := initRunStore.sessions[sessionID]
	if !ok || sess.RunCtx == nil {
		initRunStore.Unlock()
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session %s not found", sessionID))
	}
	if sess.Running {
		initRunStore.Unlock()
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session %s already has an active run", sessionID))
	}
	sess.Running = true
	initRunStore.sessions[sessionID] = sess
	initRunStore.Unlock()
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
				initRunStore.Lock()
				sess := initRunStore.sessions[sessionID]
				sess.Running = false
				initRunStore.sessions[sessionID] = sess
				initRunStore.Unlock()
				runStore.Lock()
				delete(runStore.runs, runID)
				runStore.Unlock()
				close(eventCh)
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
		initRunStore.Lock()
		sess := initRunStore.sessions[sessionID]
		sess.Running = false
		initRunStore.sessions[sessionID] = sess
		initRunStore.Unlock()
		runStore.Lock()
		delete(runStore.runs, runID)
		runStore.Unlock()
		close(eventCh)
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown worker %s", key))
	}

	// Start pipeline in background with emitter
	go func() {
		defer func() {
			initRunStore.Lock()
			sess := initRunStore.sessions[sessionID]
			sess.Running = false
			initRunStore.sessions[sessionID] = sess
			initRunStore.Unlock()
			runStore.Lock()
			delete(runStore.runs, runID)
			runStore.Unlock()
			close(eventCh)
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

func inferRepoName(repoURL string) string {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return ""
	}
	trimmed := strings.TrimSuffix(repoURL, "/")
	trimmed = strings.TrimSuffix(trimmed, ".git")
	if strings.Contains(trimmed, ":") && !strings.Contains(trimmed, "://") {
		// git@github.com:owner/repo form
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) == 2 {
			trimmed = parts[1]
		}
	}
	name := path.Base(trimmed)
	name = strings.TrimSpace(name)
	if name == "." || name == "/" {
		return ""
	}
	return name
}

func resolveSessionID(req *connect.Request[insightifyv1.StartRunRequest]) string {
	if req == nil {
		return ""
	}
	if sid := strings.TrimSpace(req.Msg.GetSessionId()); sid != "" {
		return sid
	}
	cookieHeader := req.Header().Get("Cookie")
	if cookieHeader == "" {
		return ""
	}
	for _, part := range strings.Split(cookieHeader, ";") {
		p := strings.TrimSpace(part)
		if !strings.HasPrefix(p, sessionCookieName+"=") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(p, sessionCookieName+"="))
	}
	return ""
}

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

// Ensure interface conformance
var _ insightifyv1connect.PipelineServiceHandler = (*apiServer)(nil)
