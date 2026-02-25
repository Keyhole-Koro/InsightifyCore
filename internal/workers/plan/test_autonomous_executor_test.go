package plan

import (
	"context"
	"testing"
)

func TestAutonomousExecutorRunNeedsUserInputWhenGoalEmpty(t *testing.T) {
	p := &AutonomousExecutorPipeline{}
	out, err := p.Run(context.Background(), AutonomousExecutorIn{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !out.NeedsUserAction {
		t.Fatalf("expected NeedsUserAction=true")
	}
	if out.ClientView == nil || out.ClientView.GetLlmResponse() == "" {
		t.Fatalf("expected client view response for empty goal")
	}
}

func TestAutonomousExecutorRunSelectsWorkerDAGForPlanningIntent(t *testing.T) {
	p := &AutonomousExecutorPipeline{}
	out, err := p.Run(context.Background(), AutonomousExecutorIn{
		Goal: "Please plan a worker DAG for this task",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if out.SelectedWorker != "worker_DAG" {
		t.Fatalf("expected selected worker_DAG, got %q", out.SelectedWorker)
	}
	if out.NeedsUserAction {
		t.Fatalf("expected NeedsUserAction=false")
	}
}
