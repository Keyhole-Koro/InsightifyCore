package runner

import "sort"

// MergeRegistries flattens multiple worker registries into a single resolver.
// It also computes downstream dependencies automatically from 'Requires'.
func MergeRegistries(regs ...map[string]WorkerSpec) SpecResolver {
	merged := make(map[string]WorkerSpec, 16)
	downstream := make(map[string][]string)

	for _, reg := range regs {
		for k, v := range reg {
			nk := normalizeKey(k)
			merged[nk] = v
			for _, req := range v.Requires {
				nr := normalizeKey(req)
				downstream[nr] = append(downstream[nr], nk)
			}
		}
	}

	for k, v := range merged {
		if ds, ok := downstream[k]; ok {
			sort.Strings(ds)
			v.Downstream = ds
			merged[k] = v
		}
	}

	return MapResolver{specs: merged}
}
