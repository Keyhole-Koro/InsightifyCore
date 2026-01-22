package plan

import (
	"crypto/sha1"
	"fmt"
	"sort"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

// Spec is the minimal internal planning spec derived from an external GraphSpec.
type Spec struct {
	ID           string
	RepoURL      string
	RepoPath     string
	RawSpecText  string
	Capabilities []string
}

// PhaseDescriptor is a read-only view of a pipeline phase for planning/LLM use.
type PhaseDescriptor struct {
	Key        string
	Summary    string
	Consumes   []string
	Produces   []string
	Requires   []string
	Downstream []string
	UsesLLM    bool
	Tags       []string
	Metadata   map[string]string
}

// BuildPlanFromSpec deterministically selects phases and returns an executable plan.
// It applies capability/tag filtering, ensures dependencies are included, and topologically sorts steps.
func BuildPlanFromSpec(spec Spec, descs []PhaseDescriptor) (*insightifyv1.Plan, []string) {
	now := time.Now()
	planID := fmt.Sprintf("plan-%d", now.UnixNano())
	allowed, selectAll := buildAllowedSet(spec.Capabilities, spec.RawSpecText)

	candidates := seedCandidates(descs, allowed, selectAll)
	candidates, depWarnings := ensureDependencies(descs, candidates)

	order, topoWarnings := topo(candidates)
	var steps []*insightifyv1.PlanStep
	var warnings []string

	for _, key := range order {
		d := candidates[key]
		var deps []string
		for _, dep := range d.Requires {
			if cand, ok := candidates[norm(dep)]; ok {
				deps = append(deps, cand.Key)
			} else {
				warnings = append(warnings, fmt.Sprintf("dependency %s for %s is not enabled; skipped", dep, d.Key))
			}
		}
		step := &insightifyv1.PlanStep{
			Id:          d.Key,
			Name:        strings.ToUpper(d.Key),
			PhaseKey:    d.Key,
			Kind:        insightifyv1.StepKind_STEP_KIND_PHASE,
			Description: d.Summary,
			DependsOn:   deps,
			Inputs:      bindingsFromKinds(d.Consumes, ""),
			Outputs:     bindingsFromKinds(d.Produces, d.Key),
			UsesLlm:     d.UsesLLM,
			Tools:       spec.Capabilities,
			Params: map[string]string{
				"repo_url":  spec.RepoURL,
				"repo_path": spec.RepoPath,
			},
		}
		steps = append(steps, step)
	}
	warnings = append(warnings, depWarnings...)
	warnings = append(warnings, topoWarnings...)

	meta := map[string]string{
		"repo_url":  spec.RepoURL,
		"repo_path": spec.RepoPath,
	}
	if spec.RawSpecText != "" {
		meta["prompt_sha"] = shortSHA(spec.RawSpecText)
	}

	return &insightifyv1.Plan{
		Id:       planID,
		SpecId:   spec.ID,
		Steps:    steps,
		Warnings: warnings,
		Metadata: meta,
	}, warnings
}

func bindingsFromKinds(kinds []string, producedBy string) []*insightifyv1.ArtifactBinding {
	var out []*insightifyv1.ArtifactBinding
	for _, k := range kinds {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out = append(out, &insightifyv1.ArtifactBinding{
			Name:       k,
			Kind:       k,
			ProducedBy: producedBy,
		})
	}
	return out
}

func buildAllowedSet(capabilities []string, prompt string) (map[string]struct{}, bool) {
	out := make(map[string]struct{}, len(capabilities)+4)
	for _, c := range capabilities {
		c = norm(c)
		if c == "" {
			continue
		}
		if c == "all" || c == "*" {
			return out, true
		}
		out[c] = struct{}{}
	}
	for tag := range inferTagsFromPrompt(prompt) {
		out[tag] = struct{}{}
	}
	if len(out) == 0 {
		out["mainline"] = struct{}{}
		out["codebase"] = struct{}{}
	}
	return out, false
}

func inferTagsFromPrompt(prompt string) map[string]struct{} {
	prompt = strings.ToLower(prompt)
	out := map[string]struct{}{}
	keywords := []struct {
		tag     string
		matches []string
	}{
		{tag: "codebase", matches: []string{"graph", "dependency", "call graph", "references", "ref graph"}},
		{tag: "mainline", matches: []string{"architecture", "arch", "summary", "overview"}},
		{tag: "external", matches: []string{"external", "infra", "cloud", "integration", "saas"}},
	}
	for _, k := range keywords {
		for _, m := range k.matches {
			if strings.Contains(prompt, m) {
				out[k.tag] = struct{}{}
				break
			}
		}
	}
	return out
}

func seedCandidates(descs []PhaseDescriptor, allowed map[string]struct{}, selectAll bool) map[string]PhaseDescriptor {
	candidates := map[string]PhaseDescriptor{}
	for _, d := range descs {
		if selectAll || allowedMatches(d, allowed) {
			candidates[norm(d.Key)] = d
		}
	}
	return candidates
}

func ensureDependencies(descs []PhaseDescriptor, seeds map[string]PhaseDescriptor) (map[string]PhaseDescriptor, []string) {
	index := map[string]PhaseDescriptor{}
	for _, d := range descs {
		index[norm(d.Key)] = d
	}

	out := map[string]PhaseDescriptor{}
	for k, v := range seeds {
		out[k] = v
	}

	var warnings []string
	var visit func(string)
	visit = func(key string) {
		key = norm(key)
		if key == "" {
			return
		}
		if _, ok := out[key]; ok {
			return
		}
		if d, ok := index[key]; ok {
			out[key] = d
			for _, dep := range d.Requires {
				visit(dep)
			}
		}
	}

	for _, d := range seeds {
		for _, dep := range d.Requires {
			visit(dep)
			if _, ok := out[norm(dep)]; !ok {
				warnings = append(warnings, fmt.Sprintf("missing dependency %s for %s was not found in registry", dep, d.Key))
			}
		}
	}

	return out, warnings
}

func allowedMatches(d PhaseDescriptor, allowed map[string]struct{}) bool {
	if len(allowed) == 0 {
		return true
	}
	if _, ok := allowed[norm(d.Key)]; ok {
		return true
	}
	for _, t := range d.Tags {
		if _, ok := allowed[norm(t)]; ok {
			return true
		}
	}
	return false
}

func topo(candidates map[string]PhaseDescriptor) ([]string, []string) {
	indegree := make(map[string]int)
	graph := make(map[string][]string)
	var warnings []string

	for key, d := range candidates {
		for _, dep := range d.Requires {
			nDep := norm(dep)
			if _, ok := candidates[nDep]; ok {
				indegree[key]++
				graph[nDep] = append(graph[nDep], key)
			} else {
				warnings = append(warnings, fmt.Sprintf("phase %s depends on %s which is not enabled", d.Key, dep))
			}
		}
	}

	queue := make([]string, 0, len(candidates))
	for key := range candidates {
		if indegree[key] == 0 {
			queue = append(queue, key)
		}
	}
	sort.Strings(queue)

	var order []string
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		order = append(order, key)
		for _, to := range graph[key] {
			indegree[to]--
			if indegree[to] == 0 {
				queue = append(queue, to)
			}
		}
		sort.Strings(queue)
	}

	if len(order) != len(candidates) {
		warnings = append(warnings, "cyclic or unresolved dependencies detected in registry plan")
	}
	return order, warnings
}

func shortSHA(s string) string {
	sum := sha1.Sum([]byte(s))
	return fmt.Sprintf("%x", sum)[:12]
}

func norm(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
