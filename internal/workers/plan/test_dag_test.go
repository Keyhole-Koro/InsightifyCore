package plan

import (
	"context"
	"testing"

	"insightify/internal/artifact"
)

func TestPlanContextRunBuildsGraphFromWorkers(t *testing.T) {
	p := &PlanContext{}
	in := artifact.PlanDependenciesIn{
		RepoPath: "/tmp/repo",
		Workers: []artifact.WorkerMeta{
			{
				Key:         "arch_design",
				Description: "Architecture planning",
				Requires:    []string{"code_roots"},
			},
			{
				Key:         "code_roots",
				Description: "Scan repository roots",
			},
			{
				Key:         "worker_DAG",
				Description: "Build worker DAG",
				Requires:    []string{"arch_design", "code_roots"},
			},
		},
	}

	out, err := p.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if out.ClientView == nil || out.ClientView.GetGraph() == nil {
		t.Fatalf("expected client view graph to be returned")
	}

	g := out.ClientView.GetGraph()
	if len(g.GetNodes()) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(g.GetNodes()))
	}
	if len(g.GetEdges()) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(g.GetEdges()))
	}
}

