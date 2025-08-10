package pipeline

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"insightify/internal/llm"
	t "insightify/internal/types"
)

// P5 proposes graph normalization changes via LLM (small JSON)
// and applies them deterministically to produce a stable GraphState.
type P5 struct{ LLM llm.LLMClient }

func (p *P5) Run(ctx context.Context, nodes []t.Node, edges []t.Edge, arts t.Artifacts) (t.P5Out, error) {
	// lite inputs to keep tokens small
	type nodeLite struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Kind   string   `json:"kind"`
		Layer  int      `json:"layer"`
		Origin string   `json:"origin,omitempty"`
		Paths  []string `json:"paths,omitempty"`
	}
	type edgeLite struct {
		ID   string `json:"id"`
		Src  string `json:"source"`
		Tgt  string `json:"target"`
		Type string `json:"type"`
	}
	var nl []nodeLite
	for _, n := range nodes {
		nl = append(nl, nodeLite{ID: n.ID, Name: n.Name, Kind: n.Kind, Layer: n.Layer, Origin: n.Origin, Paths: n.Paths})
	}
	var el []edgeLite
	for _, e := range edges {
		el = append(el, edgeLite{ID: e.ID, Src: e.Source, Tgt: e.Target, Type: e.Type})
	}

	// duplicate candidates
	cands := candidatePairs(nodes, edges)

	// artifacts summary (names only)
	artSummary := map[string]any{
		"schemas": func() []map[string]string {
			out := make([]map[string]string, 0, len(arts.Schemas))
			for _, s := range arts.Schemas {
				out = append(out, map[string]string{"name": s.Name, "where": s.Where})
			}
			return out
		}(),
		"config": func() []map[string]string {
			out := make([]map[string]string, 0, len(arts.Config))
			for _, c := range arts.Config {
				out = append(out, map[string]string{"key": c.Key, "where": c.Where})
			}
			return out
		}(),
		"interfaces": func() []map[string]string {
			out := make([]map[string]string, 0, len(arts.Interfaces))
			for _, i := range arts.Interfaces {
				out = append(out, map[string]string{"name": i.Name, "where": i.Where})
			}
			return out
		}(),
	}

	prompt := `You are a senior software architect.
Given a small graph (nodes_lite, edges_lite), candidate duplicate pairs, and artifacts (schemas/config/interfaces without backing nodes),
propose SAFE normalization changes.

STRICT JSON ONLY:
{
  "changes": [
    // choose from:
    // {"op":"merge_nodes","from":["node:a","node:b"],"to":"node:a","reason":"..."},
    // {"op":"promote","id":"node:x","to_kind":"subsystem","reason":"..."},
    // {"op":"demote","id":"node:y","to_kind":"module","reason":"..."},
    // {"op":"drop_node","id":"node:z","reason":"..."},
    // {"op":"drop_edge","id":"edge:...","reason":"..."},
    // {"op":"add_node","id":"node:schema:wallets","name":"Wallets","kind":"schema","layer":3,"origin":"code","paths":["src/**"],"reason":"from artifact 'wallets'"}
  ]
}

Rules:
- Only merge nodes that appear in candidates. Prefer fewer merges with clear evidence.
- to_kind must be one of: system, subsystem, module, class, schema, config.
- For add_node: allow kind only in {schema, config, interface}; layer must follow policy:
  system=0, subsystem=1, module=2, class|schema|config=3.
- Refrain from risky changes. When uncertain, omit.
- Do NOT invent new edges here. Do NOT output narrative text.`

	input := map[string]any{
		"nodes_lite": nl,
		"edges_lite": el,
		"candidates": cands,
		"artifacts":  artSummary,
		"policy": map[string]any{
			"layer_policy": map[string]int{
				"system": 0, "subsystem": 1, "module": 2, "class": 3, "schema": 3, "config": 3, "interface": 3, "function": 4,
			},
		},
	}

	raw, err := p.LLM.GenerateJSON(ctx, prompt, input)
	if err != nil {
		return t.P5Out{}, err
	}
	var resp struct {
		Changes []t.Change `json:"changes"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return t.P5Out{}, err
	}

	final := applyChanges(nodes, edges, arts, resp.Changes)
	// Clean artifacts (dedupe, keep ENV-like keys only)
	NormalizeArtifacts(&final.Artifacts)

	return t.P5Out{Changes: resp.Changes, GraphState: final}, nil
}

// --- helpers ---

// candidatePairs finds likely-duplicate node pairs.
func candidatePairs(nodes []t.Node, edges []t.Edge) []map[string]string {
	var out []map[string]string

	byName := map[string][]int{}
	for i, n := range nodes {
		k := normalizeName(n.Name)
		if k != "" {
			byName[k] = append(byName[k], i)
		}
	}
	for _, idxs := range byName {
		for i := 0; i < len(idxs); i++ {
			for j := i + 1; j < len(idxs); j++ {
				a, b := nodes[idxs[i]].ID, nodes[idxs[j]].ID
				out = append(out, map[string]string{"a": a, "b": b, "reason": "same_name"})
			}
		}
	}

	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			if overlapPaths(nodes[i].Paths, nodes[j].Paths) {
				out = append(out, map[string]string{"a": nodes[i].ID, "b": nodes[j].ID, "reason": "path_overlap"})
			}
		}
	}

	edgeSet := map[string]struct{}{}
	for _, e := range edges {
		edgeSet[e.Source+"->"+e.Target] = struct{}{}
	}
	for _, n1 := range nodes {
		for _, n2 := range nodes {
			if n1.ID == n2.ID {
				continue
			}
			_, ab := edgeSet[n1.ID+"->"+n2.ID]
			_, ba := edgeSet[n2.ID+"->"+n1.ID]
			if ab && ba {
				out = append(out, map[string]string{"a": n1.ID, "b": n2.ID, "reason": "mutual_dep"})
			}
		}
	}

	return out
}

func overlapPaths(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for _, pa := range a {
		for _, pb := range b {
			if pa == pb {
				return true
			}
			if strings.HasPrefix(pa, pb) || strings.HasPrefix(pb, pa) {
				return true
			}
		}
	}
	return false
}

func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func unionStr(a, b []string) []string {
	set := map[string]struct{}{}
	for _, s := range a {
		if s != "" {
			set[s] = struct{}{}
		}
	}
	for _, s := range b {
		if s != "" {
			set[s] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func layerForKind(kind string) int {
	switch strings.ToLower(kind) {
	case "system":
		return 0
	case "subsystem":
		return 1
	case "module":
		return 2
	case "class", "schema", "config", "interface":
		return 3
	case "function":
		return 4
	default:
		return 2
	}
}

// applyChanges applies change ops deterministically and returns a new GraphState.
// It also runs lightweight quality gates: self-loop removal and stable sorting.
func applyChanges(nodes []t.Node, edges []t.Edge, arts t.Artifacts, changes []t.Change) t.GraphState {
	// id -> index
	nidx := map[string]int{}
	for i, n := range nodes {
		nidx[n.ID] = i
	}
	alive := make([]bool, len(nodes))
	for i := range alive {
		alive[i] = true
	}

	ensureKindLayer := func(n *t.Node) { n.Layer = layerForKind(n.Kind) }

	for _, ch := range changes {
		switch ch.Op {
		case "merge_nodes":
			if ch.To == "" || len(ch.From) == 0 {
				continue
			}
			toIdx, ok := nidx[ch.To]
			if !ok || !alive[toIdx] {
				continue
			}
			for _, fid := range ch.From {
				if fid == ch.To {
					continue
				}
				fi, ok := nidx[fid]
				if !ok || !alive[fi] {
					continue
				}
				// union fields
				nodes[toIdx].Paths = unionStr(nodes[toIdx].Paths, nodes[fi].Paths)
				nodes[toIdx].Identifiers = unionStr(nodes[toIdx].Identifiers, nodes[fi].Identifiers)
				nodes[toIdx].Provenance = append(nodes[toIdx].Provenance, nodes[fi].Provenance...)
				// retarget edges
				for i := range edges {
					if edges[i].Source == fid {
						edges[i].Source = ch.To
					}
					if edges[i].Target == fid {
						edges[i].Target = ch.To
					}
				}
				alive[fi] = false
			}
			ensureKindLayer(&nodes[toIdx])

		case "promote", "demote":
			i, ok := nidx[ch.ID]
			if !ok || !alive[i] || ch.ToKind == "" {
				continue
			}
			nodes[i].Kind = strings.ToLower(ch.ToKind)
			ensureKindLayer(&nodes[i])

		case "drop_node":
			i, ok := nidx[ch.ID]
			if !ok || !alive[i] {
				continue
			}
			alive[i] = false
			for k := range edges {
				if edges[k].Source == ch.ID || edges[k].Target == ch.ID {
					edges[k].ID = "" // mark as deleted
				}
			}

		case "drop_edge":
			for k := range edges {
				if edges[k].ID == ch.ID {
					edges[k].ID = ""
				}
			}

		case "add_node":
			// only schema|config|interface
			if ch.ID == "" || ch.Kind == "" || ch.Name == "" {
				continue
			}
			if _, exists := nidx[ch.ID]; exists {
				continue
			}
			nn := t.Node{
				ID:         ch.ID,
				Name:       ch.Name,
				Kind:       strings.ToLower(ch.Kind),
				Layer:      layerForKind(ch.Kind),
				Origin:     strings.ToLower(ch.Origin),
				Paths:      append([]string(nil), ch.Paths...),
				Confidence: 0.7,
				Provenance: nil,
			}
			nodes = append(nodes, nn)
			nidx[nn.ID] = len(nodes) - 1
			alive = append(alive, true)
		}
	}

	// compact alive nodes
	var newNodes []t.Node
	newIDMap := map[string]string{}
	for i, n := range nodes {
		if alive[i] {
			newIDMap[n.ID] = n.ID
			newNodes = append(newNodes, n)
		}
	}

	// rebuild edges (drop deleted/dangling/self-loop, rebuild IDs, dedupe)
	var tmp []t.Edge
	for _, e := range edges {
		if e.ID == "" {
			continue
		}
		if _, ok := newIDMap[e.Source]; !ok {
			continue
		}
		if _, ok := newIDMap[e.Target]; !ok {
			continue
		}
		if e.Source == e.Target {
			continue
		}
		e.ID = edgeID(strings.ToLower(strings.TrimSpace(e.Type)), e.Source, e.Target) // defined in p4_post.go
		tmp = append(tmp, e)
	}
	edges = dedupeEdges(tmp) // defined in p4_post.go

	// stable sort
	sort.SliceStable(newNodes, func(i, j int) bool { return newNodes[i].ID < newNodes[j].ID })
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].Source != edges[j].Source {
			return edges[i].Source < edges[j].Source
		}
		if edges[i].Target != edges[j].Target {
			return edges[i].Target < edges[j].Target
		}
		if edges[i].Type != edges[j].Type {
			return edges[i].Type < edges[j].Type
		}
		return edges[i].ID < edges[j].ID
	})

	return t.GraphState{Nodes: newNodes, Edges: edges, Artifacts: arts}
}
