package userinteraction

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	artifactrepo "insightify/internal/gateway/repository/artifact"
)

func TestSendQueuesInputForWaitForInput(t *testing.T) {
	svc := New(nil, "")
	runID := "run-1"
	want := "hello from user"

	gotCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		in, err := svc.WaitForInput(context.Background(), runID)
		if err != nil {
			errCh <- err
			return
		}
		gotCh <- in
	}()

	select {
	case <-time.After(20 * time.Millisecond):
	case err := <-errCh:
		t.Fatalf("WaitForInput() early error = %v", err)
	}

	resp, err := svc.Send(context.Background(), &insightifyv1.SendRequest{
		RunId: runID,
		Input: want,
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !resp.GetAccepted() {
		t.Fatalf("Send() accepted = false")
	}
	if resp.GetAssistantMessage() != "" {
		t.Fatalf("Send() assistant_message = %q, want empty", resp.GetAssistantMessage())
	}

	select {
	case got := <-gotCh:
		if got != want {
			t.Fatalf("WaitForInput() got %q, want %q", got, want)
		}
	case err := <-errCh:
		t.Fatalf("WaitForInput() error = %v", err)
	case <-time.After(1 * time.Second):
		t.Fatalf("WaitForInput() timed out")
	}
}

func TestSubscribeEmitsStateTransitions(t *testing.T) {
	svc := New(nil, "")
	runID := "run-subscribe"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := svc.Subscribe(ctx, runID)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	initial := readWaitState(t, sub)
	if initial.GetWaiting() {
		t.Fatalf("initial waiting = true, want false")
	}
	if initial.GetClosed() {
		t.Fatalf("initial closed = true, want false")
	}

	waitReady := make(chan struct{})
	waitDone := make(chan struct{})
	go func() {
		close(waitReady)
		_, _ = svc.WaitForInput(context.Background(), runID)
		close(waitDone)
	}()
	<-waitReady

	_, sendErr := svc.Send(context.Background(), &insightifyv1.SendRequest{
		RunId: runID,
		Input: "hello",
	})
	if sendErr != nil {
		t.Fatalf("Send() error = %v", sendErr)
	}

	deadline := time.After(1 * time.Second)
	sawWaitingFalse := false
	for !sawWaitingFalse {
		select {
		case evt, ok := <-sub:
			if !ok {
				t.Fatalf("Subscribe channel closed unexpectedly")
			}
			if evt.Kind != SubscriptionEventWaitState || evt.WaitState == nil {
				continue
			}
			st := evt.WaitState
			if !st.GetWaiting() && !st.GetClosed() {
				sawWaitingFalse = true
			}
		case <-deadline:
			t.Fatalf("did not receive waiting=false state")
		}
	}

	select {
	case <-waitDone:
	case <-time.After(1 * time.Second):
		t.Fatalf("WaitForInput did not consume sent input")
	}
}

func TestPublishOutputEmitsAssistantMessage(t *testing.T) {
	svc := New(nil, "")
	runID := "run-output"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := svc.Subscribe(ctx, runID)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	_ = readWaitState(t, sub)

	if err := svc.PublishOutput(context.Background(), runID, "", "hello assistant"); err != nil {
		t.Fatalf("PublishOutput() error = %v", err)
	}

	deadline := time.After(1 * time.Second)
	for {
		select {
		case evt, ok := <-sub:
			if !ok {
				t.Fatalf("Subscribe channel closed unexpectedly")
			}
			if evt.Kind != SubscriptionEventAssistantMessage {
				continue
			}
			if evt.AssistantMessage != "hello assistant" {
				t.Fatalf("assistant message = %q, want %q", evt.AssistantMessage, "hello assistant")
			}
			return
		case <-deadline:
			t.Fatalf("timed out waiting for assistant message event")
		}
	}
}

func TestConversationArtifactStoredByRunID(t *testing.T) {
	store := &memoryArtifactStore{data: map[string][]byte{}}
	svc := New(store, "")
	runID := "run-conversation"
	interactionID := "interaction-1"

	_, err := svc.Send(context.Background(), &insightifyv1.SendRequest{
		RunId:         runID,
		InteractionId: interactionID,
		Input:         "hello",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if err := svc.PublishOutput(context.Background(), runID, interactionID, "hi there"); err != nil {
		t.Fatalf("PublishOutput() error = %v", err)
	}

	raw, ok := store.data[runID+"/"+svc.conversationArtifactPath]
	if !ok {
		t.Fatalf("conversation artifact not stored")
	}
	var doc struct {
		RunID    string `json:"run_id"`
		Messages []struct {
			Role          string `json:"role"`
			Content       string `json:"content"`
			InteractionID string `json:"interaction_id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal conversation artifact: %v", err)
	}
	if doc.RunID != runID {
		t.Fatalf("run_id = %q, want %q", doc.RunID, runID)
	}
	if len(doc.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(doc.Messages))
	}
	if doc.Messages[0].Role != "user" || doc.Messages[0].Content != "hello" {
		t.Fatalf("first message = %#v, want user/hello", doc.Messages[0])
	}
	if doc.Messages[1].Role != "assistant" || doc.Messages[1].Content != "hi there" {
		t.Fatalf("second message = %#v, want assistant/hi there", doc.Messages[1])
	}
	if doc.Messages[0].InteractionID != interactionID || doc.Messages[1].InteractionID != interactionID {
		t.Fatalf("interaction_id mismatch: %#v", doc.Messages)
	}
}

type memoryArtifactStore struct {
	data map[string][]byte
}

func (m *memoryArtifactStore) Put(_ context.Context, runID, path string, content []byte) error {
	if m.data == nil {
		m.data = map[string][]byte{}
	}
	m.data[runID+"/"+path] = append([]byte(nil), content...)
	return nil
}

func (m *memoryArtifactStore) Get(_ context.Context, runID, path string) ([]byte, error) {
	v, ok := m.data[runID+"/"+path]
	if !ok {
		return nil, artifactrepo.ErrNotFound
	}
	return append([]byte(nil), v...), nil
}

func (m *memoryArtifactStore) List(_ context.Context, runID string) ([]string, error) {
	return nil, nil
}

func (m *memoryArtifactStore) GetURL(ctx context.Context, runID, path string) (string, error) {
	return "", nil
}

func readWaitState(t *testing.T, sub <-chan *SubscriptionEvent) *insightifyv1.WaitResponse {
	t.Helper()
	select {
	case evt, ok := <-sub:
		if !ok {
			t.Fatalf("Subscribe channel closed")
		}
		if evt.Kind != SubscriptionEventWaitState || evt.WaitState == nil {
			t.Fatalf("unexpected subscription event kind: %q", evt.Kind)
		}
		return evt.WaitState
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for subscribe state")
		return nil
	}
}
