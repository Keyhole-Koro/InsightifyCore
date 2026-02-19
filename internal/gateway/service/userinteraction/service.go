package userinteraction

import (
	"context"
	"fmt"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	artifactrepo "insightify/internal/gateway/repository/artifact"
)

func New(artifact artifactrepo.Store, conversationArtifactPath string) *Service {
	path := strings.TrimSpace(conversationArtifactPath)
	if path == "" {
		path = defaultConversationArtifactPath
	}
	return &Service{
		state:                    make(map[string]*sessionState),
		artifact:                 artifact,
		conversationArtifactPath: path,
	}
}

func (s *Service) SetUISync(sync UISync) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uiSync = sync
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

func (s *Service) Close(_ context.Context, req *insightifyv1.CloseRequest) (*insightifyv1.CloseResponse, error) {
	runID := strings.TrimSpace(req.GetRunId())
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}

	s.mu.Lock()

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
	syncer := s.uiSync
	syncRunID := runID
	syncInter := st.interactionID
	notifyLocked(st)
	s.mu.Unlock()

	if syncer != nil {
		_ = syncer.OnWaiting(context.Background(), syncRunID, syncInter, false)
	}
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
		var (
			syncer     UISync
			syncRunID  string
			syncInter  string
			emitWaitOn bool
		)
		s.mu.Lock()
		st := s.getOrCreateLocked(runID)
		if st.interactionID == "" {
			st.interactionID = newInteractionID()
		}
		st.waiting = true
		st.updatedAt = time.Now()
		syncer = s.uiSync
		syncRunID = runID
		syncInter = st.interactionID
		emitWaitOn = syncer != nil
		if len(st.inputQueue) > 0 {
			in := strings.TrimSpace(st.inputQueue[0])
			st.inputQueue = st.inputQueue[1:]
			st.waiting = false
			st.updatedAt = time.Now()
			notifyLocked(st)
			s.mu.Unlock()
			if emitWaitOn {
				_ = syncer.OnWaiting(context.Background(), syncRunID, syncInter, true)
				_ = syncer.OnWaiting(context.Background(), syncRunID, syncInter, false)
			}
			if in == "" {
				continue
			}
			return in, nil
		}
		if st.closed {
			st.waiting = false
			notifyLocked(st)
			s.mu.Unlock()
			if emitWaitOn {
				_ = syncer.OnWaiting(context.Background(), syncRunID, syncInter, true)
				_ = syncer.OnWaiting(context.Background(), syncRunID, syncInter, false)
			}
			return "", context.Canceled
		}
		notifyLocked(st)
		ch := st.changed
		s.mu.Unlock()
		if emitWaitOn {
			_ = syncer.OnWaiting(context.Background(), syncRunID, syncInter, true)
		}

		select {
		case <-ctx.Done():
			var (
				syncer2    UISync
				syncRunID2 string
				syncInter2 string
			)
			s.mu.Lock()
			st := s.getOrCreateLocked(runID)
			st.waiting = false
			st.updatedAt = time.Now()
			syncer2 = s.uiSync
			syncRunID2 = runID
			syncInter2 = st.interactionID
			notifyLocked(st)
			s.mu.Unlock()
			if syncer2 != nil {
				_ = syncer2.OnWaiting(context.Background(), syncRunID2, syncInter2, false)
			}
			return "", ctx.Err()
		case <-ch:
		}
	}
}
