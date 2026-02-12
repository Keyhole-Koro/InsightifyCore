package main

import (
	"testing"
	"time"
)

func TestPendingUserInputRoundTrip(t *testing.T) {
	const (
		projectID = "s1"
		runID     = "r1"
		workerKey = "init_purpose"
	)
	gatewayApp.ClearUserInput(runID)

	requestID, err := gatewayApp.RegisterNeedInput(projectID, runID, workerKey, "question")
	if err != nil {
		t.Fatalf("register pending: %v", err)
	}
	if requestID == "" {
		t.Fatalf("expected request id")
	}

	gotRequestID, err := gatewayApp.SubmitUserInput(projectID, runID, "", "hello")
	if err != nil {
		t.Fatalf("submit pending: %v", err)
	}
	if gotRequestID != requestID {
		t.Fatalf("request id mismatch: got %q want %q", gotRequestID, requestID)
	}

	value, err := gatewayApp.WaitUserInput(runID, time.Second)
	if err != nil {
		t.Fatalf("wait pending: %v", err)
	}
	if value != "hello" {
		t.Fatalf("unexpected value: %q", value)
	}
}

func TestPendingUserInputTimeout(t *testing.T) {
	const (
		projectID = "s2"
		runID     = "r2"
		workerKey = "init_purpose"
	)
	gatewayApp.ClearUserInput(runID)

	if _, err := gatewayApp.RegisterNeedInput(projectID, runID, workerKey, "question"); err != nil {
		t.Fatalf("register pending: %v", err)
	}

	_, err := gatewayApp.WaitUserInput(runID, 10*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}
