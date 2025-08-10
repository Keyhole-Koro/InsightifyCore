package pipeline

import (
	"regexp"

	t "insightify/internal/types"
)

var envKeyRe = regexp.MustCompile(`^[A-Z0-9_]{2,}$`)

// NormalizeArtifacts deduplicates and cleans artifacts in-place,
// without depending on concrete element types.
// It assumes the following fields exist on each element:
//   Config:     Key, Where, Provenance
//   Schemas:    Name, Where, Provenance
//   Interfaces: Name, Where, Provenance (Kind may exist but is not required)
func NormalizeArtifacts(a *t.Artifacts) {
	// ---- config: keep ENV-like keys only, merge by key (provenance concatenated)
	outCfg := a.Config[:0]
	idxByKey := map[string]int{}
	for _, c := range a.Config {
		// Drop labels or accidental node IDs etc.
		if !envKeyRe.MatchString(c.Key) {
			continue
		}
		if i, ok := idxByKey[c.Key]; ok {
			// merge provenance; keep first Where unless empty
			outCfg[i].Provenance = append(outCfg[i].Provenance, c.Provenance...)
			if outCfg[i].Where == "" {
				outCfg[i].Where = c.Where
			}
			continue
		}
		idxByKey[c.Key] = len(outCfg)
		outCfg = append(outCfg, c)
	}
	a.Config = outCfg

	// ---- schemas: dedupe by (name, where)
	outS := a.Schemas[:0]
	seenS := map[string]struct{}{}
	for _, s := range a.Schemas {
		key := s.Name + "\x00" + s.Where
		if _, ok := seenS[key]; ok {
			continue
		}
		seenS[key] = struct{}{}
		outS = append(outS, s)
	}
	a.Schemas = outS

	// ---- interfaces: dedupe by (name, where)
	outI := a.Interfaces[:0]
	seenI := map[string]struct{}{}
	for _, it := range a.Interfaces {
		key := it.Name + "\x00" + it.Where
		if _, ok := seenI[key]; ok {
			continue
		}
		seenI[key] = struct{}{}
		outI = append(outI, it)
	}
	a.Interfaces = outI
}
