package plan

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	workerv1 "insightify/gen/go/worker/v1"
)

// AutonomousExecutorIn is the input for the fallback autonomous executor worker.
type AutonomousExecutorIn struct {
	Goal           string   `json:"goal"`
	MaxSteps       int      `json:"max_steps"`
	AllowedWorkers []string `json:"allowed_workers"`
}

// AutonomousExecutorOut is the output for the fallback autonomous executor worker.
type AutonomousExecutorOut struct {
	Goal            string               `json:"goal"`
	Plan            []string             `json:"plan"`
	SelectedWorker  string               `json:"selected_worker"`
	NeedsUserAction bool                 `json:"needs_user_action"`
	NeedsUserReason string               `json:"needs_user_reason"`
	MaxSteps        int                  `json:"max_steps"`
	AllowedWorkers  []string             `json:"allowed_workers,omitempty"`
	ClientView      *workerv1.ClientView `json:"client_view,omitempty"`
}

// AutonomousExecutorPipeline is the fallback autonomous planner used when routing confidence is low.
type AutonomousExecutorPipeline struct{}

// Run creates a deterministic execution plan from user goal text.
func (p *AutonomousExecutorPipeline) Run(_ context.Context, in AutonomousExecutorIn) (AutonomousExecutorOut, error) {
	_ = p
	goal := strings.TrimSpace(in.Goal)
	maxSteps := in.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 5
	}
	allowed := normalizeWorkers(in.AllowedWorkers)

	out := AutonomousExecutorOut{
		Goal:           goal,
		MaxSteps:       maxSteps,
		AllowedWorkers: allowed,
	}
	if goal == "" {
		out.NeedsUserAction = true
		out.NeedsUserReason = "goal is empty; additional user input is required"
		out.Plan = []string{
			"Ask user for explicit goal and expected output format.",
		}
		out.ClientView = &workerv1.ClientView{
			Phase:   "autonomous_executor",
			Content: &workerv1.ClientView_LlmResponse{LlmResponse: "I need one clear goal before I can execute autonomously."},
		}
		return out, nil
	}

	selected := selectFallbackWorker(goal)
	if len(allowed) > 0 && !containsString(allowed, selected) {
		out.NeedsUserAction = true
		out.NeedsUserReason = fmt.Sprintf("selected worker %q is not in allowed_workers", selected)
		out.Plan = []string{
			"Candidate worker is outside allowed_workers.",
			"Ask user to allow additional worker or choose from allowed set.",
		}
		out.ClientView = &workerv1.ClientView{
			Phase:   "autonomous_executor",
			Content: &workerv1.ClientView_LlmResponse{LlmResponse: out.NeedsUserReason},
		}
		return out, nil
	}

	out.SelectedWorker = selected
	out.Plan = []string{
		"Summarize objective and constraints.",
		"Select a candidate worker based on intent keywords.",
		fmt.Sprintf("Execute worker %q with bounded max_steps=%d.", selected, maxSteps),
		"Return result and request confirmation for next action.",
	}
	out.ClientView = &workerv1.ClientView{
		Phase: "autonomous_executor",
		Content: &workerv1.ClientView_LlmResponse{
			LlmResponse: fmt.Sprintf("Autonomous fallback plan prepared for goal: %s (worker=%s, max_steps=%d)", goal, selected, maxSteps),
		},
	}
	return out, nil
}

// BuildAutonomousExecutorInput builds runtime input from run params.
func BuildAutonomousExecutorInput(params map[string]string) AutonomousExecutorIn {
	goal := strings.TrimSpace(params["input"])
	if goal == "" {
		goal = strings.TrimSpace(params["goal"])
	}
	maxSteps := 5
	if v := strings.TrimSpace(params["max_steps"]); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			maxSteps = parsed
		}
	}
	return AutonomousExecutorIn{
		Goal:           goal,
		MaxSteps:       maxSteps,
		AllowedWorkers: splitCSV(params["allowed_workers"]),
	}
}

func selectFallbackWorker(goal string) string {
	g := strings.ToLower(strings.TrimSpace(goal))
	switch {
	case strings.Contains(g, "dag"), strings.Contains(g, "plan"), strings.Contains(g, "graph"):
		return "worker_DAG"
	default:
		return "bootstrap"
	}
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return normalizeWorkers(out)
}

func normalizeWorkers(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, worker := range in {
		v := strings.TrimSpace(worker)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func containsString(in []string, target string) bool {
	for _, v := range in {
		if v == target {
			return true
		}
	}
	return false
}
