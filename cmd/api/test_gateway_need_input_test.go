package main

import (
	"testing"
	"time"
)

func TestPendingUserInputRoundTrip(t *testing.T) {
	const (
		sessionID = "s1"
		runID     = "r1"
		workerKey = "init_purpose"
	)
	clearPendingUserInput(runID)

	requestID, err := registerPendingUserInput(sessionID, runID, workerKey, "question")
	if err != nil {
		t.Fatalf("register pending: %v", err)
	}
	if requestID == "" {
		t.Fatalf("expected request id")
	}

	gotRequestID, err := submitPendingUserInput(sessionID, runID, "", "hello")
	if err != nil {
		t.Fatalf("submit pending: %v", err)
	}
	if gotRequestID != requestID {
		t.Fatalf("request id mismatch: got %q want %q", gotRequestID, requestID)
	}

	value, err := waitPendingUserInput(runID, time.Second)
	if err != nil {
		t.Fatalf("wait pending: %v", err)
	}
	if value != "hello" {
		t.Fatalf("unexpected value: %q", value)
	}
}

func TestPendingUserInputTimeout(t *testing.T) {
	const (
		sessionID = "s2"
		runID     = "r2"
		workerKey = "init_purpose"
	)
	clearPendingUserInput(runID)

	if _, err := registerPendingUserInput(sessionID, runID, workerKey, "question"); err != nil {
		t.Fatalf("register pending: %v", err)
	}

	_, err := waitPendingUserInput(runID, 10*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}
