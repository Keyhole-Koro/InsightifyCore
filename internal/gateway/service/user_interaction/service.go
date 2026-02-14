package userinteraction

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

type Service struct {
	mu    sync.Mutex
	state map[string]*sessionState
}

type SubscriptionEventKind string

const (
	SubscriptionEventWaitState        SubscriptionEventKind = "wait_state"
	SubscriptionEventAssistantMessage SubscriptionEventKind = "assistant_message"
)

type SubscriptionEvent struct {
	Kind             SubscriptionEventKind
	WaitState        *insightifyv1.WaitResponse
	InteractionID    string
	AssistantMessage string
}

type outputMessage struct {
	interactionID string
	message       string
}

type sessionState struct {
	interactionID string
	closed        bool
	waiting       bool
	inputQueue    []string
	outputQueue   []outputMessage
	changed       chan struct{}
	updatedAt     time.Time
}

func New() *Service {
	return &Service{
		state: make(map[string]*sessionState),
	}
}

func (s *Service) Wait(ctx context.Context, req *insightifyv1.WaitRequest) (*insightifyv1.WaitResponse, error) {
	runID := strings.TrimSpace(req.GetRunId())
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	timeoutMs := req.GetTimeoutMs()
	waitCtx := ctx
	if timeoutMs > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(waitCtx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
	}
	for {
		s.mu.Lock()
		st := s.getOrCreateLocked(runID)
		if st.interactionID == "" {
			st.interactionID = newInteractionID()
		}
		st.updatedAt = time.Now()
		resp := &insightifyv1.WaitResponse{
			Waiting:       st.waiting && !st.closed,
			InteractionId: st.interactionID,
			Closed:        st.closed,
		}
		ch := st.changed
		ready := resp.GetWaiting() || resp.GetClosed()
		s.mu.Unlock()

		if ready || timeoutMs <= 0 {
			return resp, nil
		}
		select {
		case <-waitCtx.Done():
			return &insightifyv1.WaitResponse{
				Waiting:       false,
				InteractionId: resp.GetInteractionId(),
				Closed:        false,
			}, nil
		case <-ch:
		}
	}
}

// Snapshot returns the latest interaction wait state for a run.
func (s *Service) Snapshot(runID string) (*insightifyv1.WaitResponse, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.getOrCreateLocked(runID)
	if st.interactionID == "" {
		st.interactionID = newInteractionID()
	}
	st.updatedAt = time.Now()
	return s.waitResponseFromStateLocked(st), nil
}

// Subscribe emits interaction updates for a run until ctx is canceled.
func (s *Service) Subscribe(ctx context.Context, runID string) (<-chan *SubscriptionEvent, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	out := make(chan *SubscriptionEvent, 8)

	go func() {
		defer close(out)
		for {
			s.mu.Lock()
			st := s.getOrCreateLocked(runID)
			if st.interactionID == "" {
				st.interactionID = newInteractionID()
			}
			st.updatedAt = time.Now()
			state := s.waitResponseFromStateLocked(st)
			outputs := append([]outputMessage(nil), st.outputQueue...)
			st.outputQueue = nil
			ch := st.changed
			s.mu.Unlock()

			pushEvent(out, &SubscriptionEvent{
				Kind:      SubscriptionEventWaitState,
				WaitState: state,
			})
			for _, outMsg := range outputs {
				pushEvent(out, &SubscriptionEvent{
					Kind:             SubscriptionEventAssistantMessage,
					InteractionID:    outMsg.interactionID,
					AssistantMessage: outMsg.message,
				})
			}

			select {
			case <-ctx.Done():
				return
			case <-ch:
			}
		}
	}()

	return out, nil
}

// PublishOutput enqueues a server assistant message for the run.
func (s *Service) PublishOutput(_ context.Context, runID, interactionID, message string) error {
	runID = strings.TrimSpace(runID)
	interactionID = strings.TrimSpace(interactionID)
	message = strings.TrimSpace(message)
	if runID == "" {
		return fmt.Errorf("run_id is required")
	}
	if message == "" {
		return fmt.Errorf("message is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.getOrCreateLocked(runID)
	if interactionID != "" {
		st.interactionID = interactionID
	}
	if st.interactionID == "" {
		st.interactionID = newInteractionID()
	}
	st.outputQueue = append(st.outputQueue, outputMessage{
		interactionID: st.interactionID,
		message:       message,
	})
	st.updatedAt = time.Now()
	notifyLocked(st)
	return nil
}

func (s *Service) Send(_ context.Context, req *insightifyv1.SendRequest) (*insightifyv1.SendResponse, error) {
	runID := strings.TrimSpace(req.GetRunId())
	input := strings.TrimSpace(req.GetInput())
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	if input == "" {
		return nil, fmt.Errorf("input is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.getOrCreateLocked(runID)
	if st.closed {
		return &insightifyv1.SendResponse{
			Accepted:      false,
			InteractionId: st.interactionID,
		}, nil
	}
	if interactionID := strings.TrimSpace(req.GetInteractionId()); interactionID != "" {
		st.interactionID = interactionID
	}
	if st.interactionID == "" {
		st.interactionID = newInteractionID()
	}
	st.inputQueue = append(st.inputQueue, input)
	st.waiting = false
	st.updatedAt = time.Now()
	notifyLocked(st)

	return &insightifyv1.SendResponse{
		Accepted:         true,
		InteractionId:    st.interactionID,
		AssistantMessage: "",
	}, nil
}

func (s *Service) Close(_ context.Context, req *insightifyv1.CloseRequest) (*insightifyv1.CloseResponse, error) {
	runID := strings.TrimSpace(req.GetRunId())
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.getOrCreateLocked(runID)
	if interactionID := strings.TrimSpace(req.GetInteractionId()); interactionID != "" {
		st.interactionID = interactionID
	}
	if st.interactionID == "" {
		st.interactionID = newInteractionID()
	}
	st.closed = true
	st.waiting = false
	st.updatedAt = time.Now()
	notifyLocked(st)

	return &insightifyv1.CloseResponse{
		Closed: true,
	}, nil
}

// WaitForInput blocks until a new user input for runID is available.
func (s *Service) WaitForInput(ctx context.Context, runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", fmt.Errorf("run_id is required")
	}
	for {
		s.mu.Lock()
		st := s.getOrCreateLocked(runID)
		if st.interactionID == "" {
			st.interactionID = newInteractionID()
		}
		st.waiting = true
		st.updatedAt = time.Now()
		if len(st.inputQueue) > 0 {
			in := strings.TrimSpace(st.inputQueue[0])
			st.inputQueue = st.inputQueue[1:]
			st.waiting = false
			st.updatedAt = time.Now()
			notifyLocked(st)
			s.mu.Unlock()
			if in == "" {
				continue
			}
			return in, nil
		}
		if st.closed {
			st.waiting = false
			notifyLocked(st)
			s.mu.Unlock()
			return "", context.Canceled
		}
		notifyLocked(st)
		ch := st.changed
		s.mu.Unlock()

		select {
		case <-ctx.Done():
			s.mu.Lock()
			st := s.getOrCreateLocked(runID)
			st.waiting = false
			st.updatedAt = time.Now()
			notifyLocked(st)
			s.mu.Unlock()
			return "", ctx.Err()
		case <-ch:
		}
	}
}

func (s *Service) waitResponseFromStateLocked(st *sessionState) *insightifyv1.WaitResponse {
	if st == nil {
		return &insightifyv1.WaitResponse{}
	}
	return &insightifyv1.WaitResponse{
		Waiting:       st.waiting && !st.closed,
		InteractionId: st.interactionID,
		Closed:        st.closed,
	}
}

func (s *Service) getOrCreateLocked(runID string) *sessionState {
	key := runID
	if st, ok := s.state[key]; ok {
		return st
	}
	st := &sessionState{
		waiting: false,
		changed: make(chan struct{}),
	}
	s.state[key] = st
	return st
}

func notifyLocked(st *sessionState) {
	if st == nil {
		return
	}
	close(st.changed)
	st.changed = make(chan struct{})
}

func pushEvent(out chan *SubscriptionEvent, state *SubscriptionEvent) {
	if out == nil || state == nil {
		return
	}
	select {
	case out <- state:
		return
	default:
	}
	select {
	case <-out:
	default:
	}
	select {
	case out <- state:
	default:
	}
}

func newInteractionID() string {
	return fmt.Sprintf("interaction-%d", time.Now().UnixNano())
}
