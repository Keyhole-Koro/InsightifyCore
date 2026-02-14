package userinteraction

import (
	"context"
	"testing"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

func TestSendQueuesInputForWaitForInput(t *testing.T) {
	svc := New()
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
	svc := New()
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
	svc := New()
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
