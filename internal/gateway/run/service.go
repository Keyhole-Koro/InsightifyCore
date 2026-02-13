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
}

// New creates a run service.
func New(projects ProjectReader, uiStore *ui.Store) *Service {
	return &Service{
		projects:    projects,
		interaction: NewInteraction(),
		events:      NewEventBroker(),
		uiStore:     uiStore,
	}
}

// Interaction returns the interaction manager (needed for wiring).
func (s *Service) Interaction() *Interaction { return s.interaction }

// ---------------------------------------------------------------------------
// RunServiceHandler RPC implementations
// ---------------------------------------------------------------------------

func (s *Service) StartRun(ctx context.Context, req *connect.Request[insightifyv1.StartRunRequest]) (*connect.Response[insightifyv1.StartRunResponse], error) {
	projectID, workerKey, userInput, err := s.prepareStartRun(req)
	if err != nil {
		return nil, err
	}
	runID, err := s.launchWorkerRun(projectID, workerKey, userInput)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to start %s: %w", workerKey, err))
	}
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

	gotInteractionID, err := s.interaction.SubmitUserInput(projectID, runID, interactionID, input)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

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
				return nil
			}
			if err := stream.Send(event); err != nil {
				return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send event: %w", err))
			}
			if event.EventType == insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE || event.EventType == insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR {
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
	s.interaction.MarkRunStarted(projectID, runID)
	eventCh := s.events.Allocate(runID, 128)

	sess := WorkerSession{ProjectID: projectID, Env: runCtx.Env}

	go func() {
		defer func() {
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
