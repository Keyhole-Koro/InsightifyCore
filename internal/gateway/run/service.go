package run

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/runtime"
	"insightify/internal/gateway/ui"
	internalui "insightify/internal/ui"

	"connectrpc.com/connect"
)

// ProjectReader provides read access to project state for run operations.
type ProjectReader interface {
	GetEntry(projectID string) (ProjectView, bool)
	EnsureRunContext(projectID string) (runtime.RunEnvironment, error)
}

// ProjectView is a minimal view of project state needed by run operations.
type ProjectView struct {
	ProjectID string
	RunCtx    runtime.RunEnvironment
}

// Service implements RunServiceHandler and owns all run-related state.
type Service struct {
	projects    ProjectReader
	interaction *Interaction
	events      *EventBroker
	uiStore     *ui.Store
	tracer      *TraceLogger
}

// New creates a run service.
func New(projects ProjectReader, uiStore *ui.Store) *Service {
	tracer := newTraceLogger(defaultRunTraceDir())
	return &Service{
		projects:    projects,
		interaction: NewInteraction(tracer),
		events:      NewEventBroker(),
		uiStore:     uiStore,
		tracer:      tracer,
	}
}

// Interaction returns the interaction manager (needed for wiring).
func (s *Service) Interaction() *Interaction { return s.interaction }

// TraceLogger returns the structured run trace logger.
func (s *Service) TraceLogger() *TraceLogger { return s.tracer }

func (s *Service) trace(runID, source, stage string, fields map[string]any) {
	if s == nil || s.tracer == nil {
		return
	}
	s.tracer.Append(runID, source, stage, fields)
}

// ---------------------------------------------------------------------------
// RunServiceHandler RPC implementations
// ---------------------------------------------------------------------------

func (s *Service) StartRun(ctx context.Context, req *connect.Request[insightifyv1.StartRunRequest]) (*connect.Response[insightifyv1.StartRunResponse], error) {
	projectID, workerKey, userInput, err := s.prepareStartRun(req)
	if err != nil {
		log.Printf("[run] StartRun invalid request: err=%v", err)
		return nil, err
	}
	s.trace("", "api.start_run", "request", map[string]any{
		"project_id":     projectID,
		"worker":         workerKey,
		"user_input_len": len(userInput),
	})
	log.Printf("[run] StartRun request: project_id=%s worker=%s user_input_len=%d", projectID, workerKey, len(userInput))
	runID, err := s.launchWorkerRun(projectID, workerKey, userInput)
	if err != nil {
		log.Printf("[run] StartRun launch failed: project_id=%s worker=%s err=%v", projectID, workerKey, err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to start %s: %w", workerKey, err))
	}
	s.trace(runID, "api.start_run", "started", map[string]any{
		"project_id": projectID,
		"worker":     workerKey,
	})
	log.Printf("[run] StartRun started: project_id=%s worker=%s run_id=%s", projectID, workerKey, runID)
	return connect.NewResponse(&insightifyv1.StartRunResponse{RunId: runID}), nil
}

func (s *Service) SubmitInput(_ context.Context, req *connect.Request[insightifyv1.SubmitInputRequest]) (*connect.Response[insightifyv1.SubmitInputResponse], error) {
	projectID := strings.TrimSpace(req.Msg.GetProjectId())
	runID := strings.TrimSpace(req.Msg.GetRunId())
	input := strings.TrimSpace(req.Msg.GetInput())
	interactionID := strings.TrimSpace(req.Msg.GetInteractionId())
	conversationID := strings.TrimSpace(req.Msg.GetConversationId())

	if projectID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
	}
	if input == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("input is required"))
	}
	if runID == "" {
		runID = strings.TrimSpace(s.interaction.ActiveRunID(projectID))
	}
	if runID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("run_id is required"))
	}
	if conversationID == "" {
		conversationID = s.interaction.ConversationIDByRun(runID)
	}
	log.Printf(
		"[run] SubmitInput request: project_id=%s run_id=%s interaction_id=%s conversation_id=%s input_len=%d",
		projectID, runID, interactionID, conversationID, len(input),
	)
	s.trace(runID, "api.submit_input", "request", map[string]any{
		"project_id":      projectID,
		"interaction_id":  interactionID,
		"conversation_id": conversationID,
		"input_len":       len(input),
	})

	gotInteractionID, err := s.interaction.SubmitUserInput(projectID, runID, interactionID, input)
	if err != nil {
		log.Printf("[run] SubmitInput failed: project_id=%s run_id=%s err=%v", projectID, runID, err)
		s.trace(runID, "api.submit_input", "failed", map[string]any{
			"project_id": projectID,
			"error":      err.Error(),
		})
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	log.Printf(
		"[run] SubmitInput accepted: project_id=%s run_id=%s interaction_id=%s conversation_id=%s",
		projectID, runID, gotInteractionID, conversationID,
	)
	s.trace(runID, "api.submit_input", "accepted", map[string]any{
		"project_id":      projectID,
		"interaction_id":  gotInteractionID,
		"conversation_id": conversationID,
	})

	return connect.NewResponse(&insightifyv1.SubmitInputResponse{
		Accepted:       true,
		RunId:          runID,
		InteractionId:  gotInteractionID,
		ConversationId: conversationID,
	}), nil
}

func (s *Service) WatchRun(ctx context.Context, req *connect.Request[insightifyv1.WatchRunRequest], stream *connect.ServerStream[insightifyv1.WatchRunResponse]) error {
	runID := req.Msg.GetRunId()
	eventCh, ok := s.events.Get(runID)
	log.Printf("[run] WatchRun open: run_id=%s", runID)
	s.trace(runID, "api.watch_run", "open", nil)
	if !ok {
		log.Printf("[run] WatchRun missing run: run_id=%s", runID)
		s.trace(runID, "api.watch_run", "missing_run", nil)
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("run %s not found", runID))
	}

	// Emit immediate snapshot so reconnect watchers can recover latest state.
	if pending, exists := s.interaction.GetPending(runID); exists {
		snapshot := &insightifyv1.WatchRunResponse{
			EventType:      insightifyv1.WatchRunResponse_EVENT_TYPE_INPUT_REQUIRED,
			InputRequestId: pending.InteractionID,
		}
		if node := s.uiStore.Get(runID); node != nil {
			snapshot.Node = node
		}
		log.Printf("[run] WatchRun snapshot pending: run_id=%s interaction_id=%s node=%t", runID, pending.InteractionID, snapshot.GetNode() != nil)
		s.trace(runID, "api.watch_run", "snapshot_pending", map[string]any{
			"interaction_id": pending.InteractionID,
			"has_node":       snapshot.GetNode() != nil,
		})
		if err := stream.Send(snapshot); err != nil {
			log.Printf("[run] WatchRun snapshot send failed: run_id=%s err=%v", runID, err)
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send snapshot: %w", err))
		}
	} else if node := s.uiStore.Get(runID); node != nil {
		log.Printf("[run] WatchRun snapshot node: run_id=%s node_id=%s", runID, node.GetId())
		s.trace(runID, "api.watch_run", "snapshot_node", map[string]any{
			"node_id": node.GetId(),
		})
		if err := stream.Send(&insightifyv1.WatchRunResponse{
			EventType: insightifyv1.WatchRunResponse_EVENT_TYPE_NODE_READY,
			Node:      node,
		}); err != nil {
			log.Printf("[run] WatchRun snapshot send failed: run_id=%s err=%v", runID, err)
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send snapshot: %w", err))
		}
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("[run] WatchRun closed by context: run_id=%s", runID)
			s.trace(runID, "api.watch_run", "closed_by_context", nil)
			return nil
		case event, ok := <-eventCh:
			if !ok {
				log.Printf("[run] WatchRun event channel closed: run_id=%s", runID)
				s.trace(runID, "api.watch_run", "channel_closed", nil)
				return nil
			}
			log.Printf("[run] WatchRun event: run_id=%s event_type=%s msg_len=%d input_request_id=%s node=%t", runID, event.EventType.String(), len(strings.TrimSpace(event.GetMessage())), event.GetInputRequestId(), event.GetNode() != nil)
			s.trace(runID, "api.watch_run", "event", map[string]any{
				"event_type":       event.EventType.String(),
				"message_len":      len(strings.TrimSpace(event.GetMessage())),
				"input_request_id": event.GetInputRequestId(),
				"has_node_payload": event.GetNode() != nil,
				"progress_percent": event.GetProgressPercent(),
			})
			if err := stream.Send(event); err != nil {
				log.Printf("[run] WatchRun send failed: run_id=%s err=%v", runID, err)
				s.trace(runID, "api.watch_run", "send_failed", map[string]any{
					"error": err.Error(),
				})
				return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send event: %w", err))
			}
			if event.EventType == insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE || event.EventType == insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR {
				log.Printf("[run] WatchRun terminal event: run_id=%s event_type=%s", runID, event.EventType.String())
				s.trace(runID, "api.watch_run", "terminal", map[string]any{
					"event_type": event.EventType.String(),
				})
				return nil
			}
		}
	}
}

// ---------------------------------------------------------------------------
// request preparation
// ---------------------------------------------------------------------------

func (s *Service) prepareStartRun(req *connect.Request[insightifyv1.StartRunRequest]) (projectID, workerKey, userInput string, err error) {
	projectID = strings.TrimSpace(req.Msg.GetProjectId())
	if projectID == "" {
		return "", "", "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
	}
	ps, ok := s.projects.GetEntry(projectID)
	if !ok {
		return "", "", "", connect.NewError(connect.CodeNotFound, fmt.Errorf("project %s not found", projectID))
	}
	_ = ps
	if _, err := s.projects.EnsureRunContext(projectID); err != nil {
		return "", "", "", connect.NewError(connect.CodeInternal, err)
	}
	return projectID, strings.TrimSpace(req.Msg.GetPipelineId()), strings.TrimSpace(req.Msg.GetParams()["user_input"]), nil
}

// ---------------------------------------------------------------------------
// worker launch & execution
// ---------------------------------------------------------------------------

func (s *Service) launchWorkerRun(projectID, workerKey, userInput string) (string, error) {
	ps, ok := s.projects.GetEntry(projectID)
	if !ok || ps.RunCtx == nil {
		return "", fmt.Errorf("project %s not found", projectID)
	}
	runCtx, _ := ps.RunCtx.(*runtime.RunContext)
	if runCtx == nil || runCtx.Env == nil || runCtx.Env.Resolver == nil {
		return "", fmt.Errorf("project %s has no valid run context", projectID)
	}

	runID := fmt.Sprintf("%s-%d", workerKey, time.Now().UnixNano())
	log.Printf("[run] launchWorkerRun: project_id=%s worker=%s run_id=%s user_input_len=%d", projectID, workerKey, runID, len(strings.TrimSpace(userInput)))
	s.trace(runID, "svc.launch_worker", "allocated", map[string]any{
		"project_id":     projectID,
		"worker":         workerKey,
		"user_input_len": len(strings.TrimSpace(userInput)),
	})
	s.interaction.MarkRunStarted(projectID, runID)
	eventCh := s.events.Allocate(runID, 128)

	sess := WorkerSession{ProjectID: projectID, Env: runCtx.Env}

	go func() {
		defer func() {
			log.Printf("[run] launchWorkerRun cleanup: project_id=%s worker=%s run_id=%s", projectID, workerKey, runID)
			s.trace(runID, "svc.launch_worker", "cleanup", map[string]any{
				"project_id": projectID,
				"worker":     workerKey,
			})
			s.interaction.Clear(runID)
			s.uiStore.Clear(runID)
			s.interaction.MarkRunFinished(projectID, runID)
			close(eventCh)
			s.events.ScheduleCleanup(runID)
		}()
		s.executeWorkerRun(sess, runID, workerKey, userInput, eventCh)
	}()

	return runID, nil
}

// toProtoUINode converts a internal ui.Node to its proto representation.
func toProtoUINode(node internalui.Node) *insightifyv1.UiNode {
	if strings.TrimSpace(node.ID) == "" {
		return nil
	}
	return internalui.ToProtoNode(node)
}
