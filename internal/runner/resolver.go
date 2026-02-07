package runner

import (
	"sort"
)

// SpecResolver resolves worker keys to specs, enabling cross-registry lookup.
type SpecResolver interface {
	Get(key string) (WorkerSpec, bool)
	List() []WorkerSpec
}

// MapResolver is a simple SpecResolver backed by a map keyed by normalized worker keys.
type MapResolver struct {
	specs map[string]WorkerSpec
}

// Get returns the WorkerSpec for the provided key, if present.
func (r MapResolver) Get(key string) (WorkerSpec, bool) {
	if len(r.specs) == 0 {
		return WorkerSpec{}, false
	}
	spec, ok := r.specs[normalizeKey(key)]
	return spec, ok
}

// List returns all registered worker specs.
func (r MapResolver) List() []WorkerSpec {
	specs := make([]WorkerSpec, 0, len(r.specs))
	for _, s := range r.specs {
		specs = append(specs, s)
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Key < specs[j].Key })
	return specs
}

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

	// Update downstream fields in specs
	for k, v := range merged {
		if ds, ok := downstream[k]; ok {
			// Sort for determinism
			sort.Strings(ds)
			v.Downstream = ds
			merged[k] = v
		}
	}

	return MapResolver{specs: merged}
}
