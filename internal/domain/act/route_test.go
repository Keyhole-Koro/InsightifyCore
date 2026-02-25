package act

import "testing"

func TestRouteInput_Suggest(t *testing.T) {
	cases := []string{
		"suggest a good approach",
		"propose a design",
		"提案してください",
		"おすすめの方法",
	}
	for _, input := range cases {
		r := RouteInput(input)
		if r.Mode != "suggest" {
			t.Errorf("RouteInput(%q).Mode = %q, want suggest", input, r.Mode)
		}
		if r.Confidence < 0.7 {
			t.Errorf("RouteInput(%q).Confidence = %f, want >= 0.7", input, r.Confidence)
		}
	}
}

func TestRouteInput_Search(t *testing.T) {
	cases := []string{
		"search for relevant files",
		"find the bug",
		"検索してください",
		"探して",
	}
	for _, input := range cases {
		r := RouteInput(input)
		if r.Mode != "search" {
			t.Errorf("RouteInput(%q).Mode = %q, want search", input, r.Mode)
		}
		if r.Confidence < 0.7 {
			t.Errorf("RouteInput(%q).Confidence = %f, want >= 0.7", input, r.Confidence)
		}
	}
}

func TestRouteInput_RunWorker(t *testing.T) {
	cases := []string{
		"refactor the authentication module",
		"fix this error",
		"implement the feature",
	}
	for _, input := range cases {
		r := RouteInput(input)
		if r.Mode != "run_worker" {
			t.Errorf("RouteInput(%q).Mode = %q, want run_worker", input, r.Mode)
		}
	}
}

func TestRouteInput_Ambiguous(t *testing.T) {
	// "search" + "suggest" both match → ambiguous → low confidence
	input := "search and suggest options"
	r := RouteInput(input)
	if r.Confidence >= 0.5 {
		t.Errorf("RouteInput(%q).Confidence = %f, expected < 0.5 for ambiguous", input, r.Confidence)
	}
}

func TestRouteInput_Empty(t *testing.T) {
	r := RouteInput("")
	if r.Confidence >= 0.5 {
		t.Errorf("RouteInput(\"\").Confidence = %f, expected < 0.5", r.Confidence)
	}
	if r.WorkerKey != "bootstrap" {
		t.Errorf("RouteInput(\"\").WorkerKey = %q, want bootstrap", r.WorkerKey)
	}
}

func TestShouldFallback(t *testing.T) {
	if !ShouldFallback(0.3) {
		t.Error("expected fallback at confidence 0.3")
	}
	if !ShouldFallback(0.49) {
		t.Error("expected fallback at confidence 0.49")
	}
	if ShouldFallback(0.5) {
		t.Error("should not fallback at confidence 0.5")
	}
	if ShouldFallback(0.8) {
		t.Error("should not fallback at confidence 0.8")
	}
}

func TestIsWorkerAllowed(t *testing.T) {
	// Empty allowed list → all allowed
	if !IsWorkerAllowed("bootstrap", nil) {
		t.Error("expected all workers allowed when list is nil")
	}
	if !IsWorkerAllowed("bootstrap", []string{}) {
		t.Error("expected all workers allowed when list is empty")
	}

	// Explicit list
	allowed := []string{"bootstrap", "worker_DAG"}
	if !IsWorkerAllowed("bootstrap", allowed) {
		t.Error("expected bootstrap to be allowed")
	}
	if !IsWorkerAllowed("worker_DAG", allowed) {
		t.Error("expected worker_DAG to be allowed")
	}
	if IsWorkerAllowed("autonomous_executor", allowed) {
		t.Error("expected autonomous_executor to be denied")
	}

	// Case insensitive
	if !IsWorkerAllowed("Bootstrap", allowed) {
		t.Error("expected case-insensitive match")
	}
}
