package runner

import (
	"fmt"
	"strings"
)

// WorkerDescriptor is a simplified view of a WorkerSpec for visualization.
type WorkerDescriptor struct {
	Key      string
	Summary  string
	Requires []string
}

// BuildWorkerDescriptors aggregates all known workers for visualization.
func BuildWorkerDescriptors() []WorkerDescriptor {
	env := &Env{}
	// Collect all registries
	regs := []map[string]WorkerSpec{
		BuildRegistryArchitecture(env),
		BuildRegistryCodebase(env),
		BuildRegistryExternal(env),
		BuildRegistryPlanDependencies(env),
	}

	resolver := MergeRegistries(regs...)
	specs := resolver.List()

	descs := make([]WorkerDescriptor, 0, len(specs))
	for _, s := range specs {
		descs = append(descs, WorkerDescriptor{
			Key:      s.Key,
			Summary:  s.Description,
			Requires: s.Requires,
		})
	}
	return descs
}

// GenerateMermaidGraph returns a Mermaid flowchart string of the pipeline workers.
func GenerateMermaidGraph() string {
	descs := BuildWorkerDescriptors()
	var sb strings.Builder
	// Enable HTML labels so we can size the key/summary independently.
	sb.WriteString("%%{init: {'flowchart': {'htmlLabels': true}, 'themeVariables': {'fontSize': '14px'}}}%%\n")
	sb.WriteString("graph TD\n")

	// 1. Nodes
	for _, d := range descs {
		// Escape double quotes in summary to avoid breaking mermaid syntax
		summary := strings.ReplaceAll(d.Summary, "\"", "'")
		sb.WriteString(fmt.Sprintf(
			"  %s[\"<span style='font-size:16px;font-weight:600'>%s</span><br/><span style='font-size:12px'>%s</span>\"]\n",
			d.Key,
			d.Key,
			summary,
		))
	}
	sb.WriteString("\n")

	// 2. Edges
	for _, d := range descs {
		for _, req := range d.Requires {
			sb.WriteString(fmt.Sprintf("  %s --> %s\n", req, d.Key))
		}
	}

	return sb.String()
}
