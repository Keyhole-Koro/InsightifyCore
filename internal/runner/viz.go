package runner

import (
	"fmt"
	"strings"
)

// GenerateMermaidGraph returns a Mermaid flowchart string of the pipeline phases.
func GenerateMermaidGraph() string {
	descs := BuildPhaseDescriptors()
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
