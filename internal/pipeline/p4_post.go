// internal/pipeline/p4_post.go
package pipeline

import (
	"sort"
	"strings"

	t "insightify/internal/types"
)

// PostProcessP4 canonicalizes a P4Out:
// - Rebuild edge IDs as "edge:<type>:<source>-><target>"
// - Drop edges whose source/target nodes do not exist
// - Prefer evidence file paths for artifacts.where
// - Dedupe edges (merge evidence, blend confidence)
func PostProcessP4(out *t.P4Out, nodes []t.Node) {
	// Build node set
	nodeSet := make(map[string]struct{}, len(nodes))
	for _, n := range nodes {
		nodeSet[n.ID] = struct{}{}
	}

	// Canonicalize edges
	var kept []t.Edge
	for _, e := range out.Edges {
		e.Source = strings.TrimSpace(e.Source)
		e.Target = strings.TrimSpace(e.Target)
		e.Type = strings.ToLower(strings.TrimSpace(e.Type))
		// Rebuild canonical ID (drop any verb suffix that slipped into ID)
		e.ID = edgeID(e.Type, e.Source, e.Target)
		// Drop dangling
		if _, ok := nodeSet[e.Source]; !ok { continue }
		if _, ok := nodeSet[e.Target]; !ok { continue }
		kept = append(kept, e)
	}
	out.Edges = dedupeEdges(kept)

	// Artifacts.where -> prefer evidence file if where looks like a node id or is empty
	for i := range out.Artifacts.Config {
		if needsWhereFix(out.Artifacts.Config[i].Where) && len(out.Artifacts.Config[i].Provenance) > 0 {
			out.Artifacts.Config[i].Where = out.Artifacts.Config[i].Provenance[0].File
		}
	}
	for i := range out.Artifacts.Schemas {
		if needsWhereFix(out.Artifacts.Schemas[i].Where) && len(out.Artifacts.Schemas[i].Provenance) > 0 {
			out.Artifacts.Schemas[i].Where = out.Artifacts.Schemas[i].Provenance[0].File
		}
	}
	for i := range out.Artifacts.Interfaces {
		if needsWhereFix(out.Artifacts.Interfaces[i].Where) && len(out.Artifacts.Interfaces[i].Provenance) > 0 {
			out.Artifacts.Interfaces[i].Where = out.Artifacts.Interfaces[i].Provenance[0].File
		}
	}
}

func needsWhereFix(where string) bool {
	if where == "" { return true }
	// Heuristic: looks like a node id
	return strings.HasPrefix(where, "node:")
}

func edgeID(typ, src, tgt string) string {
	return "edge:" + typ + ":" + src + "->" + tgt
}

func dedupeEdges(in []t.Edge) []t.Edge {
	seen := map[string]int{}
	var out []t.Edge
	for _, e := range in {
		key := e.ID
		if idx, ok := seen[key]; ok {
			// merge evidence and blend confidence
			out[idx].Evidence = append(out[idx].Evidence, e.Evidence...)
			out[idx].Confidence = (out[idx].Confidence + e.Confidence) / 2
			// shallow-merge attrs (keep existing keys)
			if out[idx].Attrs == nil { out[idx].Attrs = map[string]string{} }
			for k, v := range e.Attrs {
				if _, exists := out[idx].Attrs[k]; !exists && v != "" {
					out[idx].Attrs[k] = v
				}
			}
		} else {
			seen[key] = len(out)
			out = append(out, e)
		}
	}
	// stable sort
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Source != out[j].Source { return out[i].Source < out[j].Source }
		if out[i].Target != out[j].Target { return out[i].Target < out[j].Target }
		if out[i].Type != out[j].Type { return out[i].Type < out[j].Type }
		return out[i].ID < out[j].ID
	})
	return out
}
