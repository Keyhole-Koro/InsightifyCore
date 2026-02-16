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
