package runner

import "sync"

// RegistryBuilder builds a registry of workers given a runtime.
type RegistryBuilder func(Runtime) map[string]WorkerSpec

var (
	registryBuildersMu sync.Mutex
	registryBuilders   []RegistryBuilder
)

// RegisterBuilder registers a new registry builder.
// It should be called in an init() function.
func RegisterBuilder(b RegistryBuilder) {
	registryBuildersMu.Lock()
	defer registryBuildersMu.Unlock()
	registryBuilders = append(registryBuilders, b)
}

// BuildAllRegistries builds and merges all registered registries.
func BuildAllRegistries(r Runtime) SpecResolver {
	registryBuildersMu.Lock()
	builders := make([]RegistryBuilder, len(registryBuilders))
	copy(builders, registryBuilders)
	registryBuildersMu.Unlock()

	registries := make([]map[string]WorkerSpec, 0, len(builders))
	for _, builder := range builders {
		registries = append(registries, builder(r))
	}
	return MergeRegistries(registries...)
}
