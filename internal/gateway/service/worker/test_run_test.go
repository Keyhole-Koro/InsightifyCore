package worker

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

type testProjectReader struct{}

func (testProjectReader) GetEntry(projectID string) (ProjectView, bool) {
	return ProjectView{ProjectID: projectID}, true
}

func (testProjectReader) EnsureRunContext(projectID string) (*ProjectRuntime, error) {
	return nil, fmt.Errorf("test: no runtime for %s", projectID)
}

type testWorkspaceRunBinder struct {
	mu      sync.Mutex
	calls   int
	project string
	runID   string
}

func (b *testWorkspaceRunBinder) AssignRunToCurrentTab(projectID, runID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	b.project = projectID
	b.runID = runID
	return nil
}

func (b *testWorkspaceRunBinder) snapshot() (calls int, project string, runID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls, b.project, b.runID
}

func TestStartRunAssignsRunToCurrentTabImmediately(t *testing.T) {
	project := testProjectReader{}
	binder := &testWorkspaceRunBinder{}
	svc := New(project, binder, nil, nil, nil)

	res, err := svc.StartRun(context.Background(), &insightifyv1.StartRunRequest{
		ProjectId: "project-1",
		WorkerId:  "testllmChatNode",
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	runID := res.GetRunId()
	if runID == "" {
		t.Fatalf("run_id is empty")
	}

	// StartRun should assign immediately; wait a tiny bit to avoid scheduler timing flake.
	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		calls, pid, assignedRunID := binder.snapshot()
		if calls > 0 {
			if pid != "project-1" {
				t.Fatalf("project_id = %q, want %q", pid, "project-1")
			}
			if assignedRunID != runID {
				t.Fatalf("assigned run_id = %q, want %q", assignedRunID, runID)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("AssignRunToCurrentTab was not called")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
