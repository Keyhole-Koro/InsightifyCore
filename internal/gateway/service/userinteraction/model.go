package userinteraction

import (
	"context"
	"fmt"
	"sync"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	artifactrepo "insightify/internal/gateway/repository/artifact"
)

const defaultConversationArtifactPath = "interaction/conversation_history.json"

type Service struct {
	mu                       sync.Mutex
	state                    map[string]*sessionState
	artifact                 artifactrepo.Store
	conversationArtifactPath string
	uiSync                   UISync
}

// UISync updates UiDocument from interaction events on the core side.
type UISync interface {
	OnUserAccepted(ctx context.Context, runID, interactionID, input string) error
	OnAssistantOutput(ctx context.Context, runID, interactionID, message string) error
	OnWaiting(ctx context.Context, runID, interactionID string, waiting bool) error
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

type conversationMessage struct {
	Seq             int    `json:"seq"`
	Role            string `json:"role"`
	Content         string `json:"content"`
	InteractionID   string `json:"interaction_id,omitempty"`
	CreatedAtUnixMs int64  `json:"created_at_unix_ms"`
}

type conversationArtifact struct {
	RunID    string                `json:"run_id"`
	Messages []conversationMessage `json:"messages"`
}

type sessionState struct {
	interactionID string
	closed        bool
	waiting       bool
	inputQueue    []string
	outputQueue   []outputMessage
	conversation  []conversationMessage
	changed       chan struct{}
	updatedAt     time.Time
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
