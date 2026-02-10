package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type pendingUserInput struct {
	RequestID string
	SessionID string
	RunID     string
	WorkerKey string
	Prompt    string
	inputCh   chan string
	done      chan struct{}
	closeOnce sync.Once
}

func (p *pendingUserInput) closeDone() {
	p.closeOnce.Do(func() {
		close(p.done)
	})
}

var pendingInputStore = struct {
	sync.RWMutex
	byRun map[string]*pendingUserInput
}{
	byRun: make(map[string]*pendingUserInput),
}

func preparePendingRegistration(sessionID, runID, workerKey string) (string, string, string, error) {
	// Normalize registration identifiers to avoid mismatched map keys from whitespace inputs.
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	workerKey = strings.TrimSpace(workerKey)
	if sessionID == "" || runID == "" || workerKey == "" {
		return "", "", "", fmt.Errorf("invalid pending input registration")
	}
	return sessionID, runID, workerKey, nil
}

func normalizePendingRunID(runID string) (string, error) {
	// Normalize run_id before any store lookup so all pending operations share the same key format.
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", fmt.Errorf("run_id is required")
	}
	return runID, nil
}

func preparePendingSubmission(sessionID, runID, input string) (string, string, string, error) {
	// Normalize submission payload to keep session/run matching and input validation consistent.
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	input = strings.TrimSpace(input)
	if sessionID == "" || runID == "" {
		return "", "", "", fmt.Errorf("session_id and run_id are required")
	}
	if input == "" {
		return "", "", "", fmt.Errorf("input is required")
	}
	return sessionID, runID, input, nil
}

func registerPendingUserInput(sessionID, runID, workerKey, prompt string) (string, error) {
	var err error
	sessionID, runID, workerKey, err = preparePendingRegistration(sessionID, runID, workerKey)
	if err != nil {
		return "", err
	}

	pendingInputStore.Lock()
	defer pendingInputStore.Unlock()

	if _, exists := pendingInputStore.byRun[runID]; exists {
		return "", fmt.Errorf("run %s already waiting for user input", runID)
	}

	requestID := fmt.Sprintf("input-%d", time.Now().UnixNano())
	pendingInputStore.byRun[runID] = &pendingUserInput{
		RequestID: requestID,
		SessionID: sessionID,
		RunID:     runID,
		WorkerKey: workerKey,
		Prompt:    strings.TrimSpace(prompt),
		inputCh:   make(chan string, 1),
		done:      make(chan struct{}),
	}
	return requestID, nil
}

func waitPendingUserInput(runID string, timeout time.Duration) (string, error) {
	var err error
	runID, err = normalizePendingRunID(runID)
	if err != nil {
		return "", err
	}

	pendingInputStore.RLock()
	pending, ok := pendingInputStore.byRun[runID]
	pendingInputStore.RUnlock()
	if !ok {
		return "", fmt.Errorf("run %s is not waiting for input", runID)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-pending.done:
		return "", fmt.Errorf("run %s input wait canceled", runID)
	case value := <-pending.inputCh:
		clearPendingUserInput(runID)
		return strings.TrimSpace(value), nil
	case <-timer.C:
		clearPendingUserInput(runID)
		return "", fmt.Errorf("run %s input wait timed out", runID)
	}
}

func submitPendingUserInput(sessionID, runID, input string) (string, error) {
	var err error
	sessionID, runID, input, err = preparePendingSubmission(sessionID, runID, input)
	if err != nil {
		return "", err
	}

	pendingInputStore.RLock()
	pending, ok := pendingInputStore.byRun[runID]
	pendingInputStore.RUnlock()
	if !ok {
		return "", fmt.Errorf("run %s is not waiting for input", runID)
	}
	if pending.SessionID != sessionID {
		return "", fmt.Errorf("run %s does not belong to session %s", runID, sessionID)
	}

	select {
	case <-pending.done:
		return "", fmt.Errorf("run %s input wait canceled", runID)
	case pending.inputCh <- input:
		return pending.RequestID, nil
	default:
		return "", fmt.Errorf("run %s already received pending input", runID)
	}
}

func clearPendingUserInput(runID string) {
	var err error
	runID, err = normalizePendingRunID(runID)
	if err != nil {
		return
	}

	pendingInputStore.Lock()
	pending, ok := pendingInputStore.byRun[runID]
	if ok {
		delete(pendingInputStore.byRun, runID)
	}
	pendingInputStore.Unlock()

	if ok {
		pending.closeDone()
	}
}
