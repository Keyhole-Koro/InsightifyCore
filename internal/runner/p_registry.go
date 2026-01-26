package runner

import (
	"sort"

	"insightify/internal/pipeline/plan"
)

// BuildPhaseDescriptors collects phase descriptors across all registries for planning.
func BuildPhaseDescriptors() []plan.PhaseDescriptor {
	env := &Env{}
	regs := []map[string]PhaseSpec{
		BuildRegistryArchitecture(env),
		BuildRegistryCodebase(env),
		BuildRegistryExternal(env),
	}
	var descs []plan.PhaseDescriptor
	for _, reg := range regs {
		descs = append(descs, DescribeRegistry(reg)...)
	}
	sort.Slice(descs, func(i, j int) bool { return descs[i].Key < descs[j].Key })
	return descs
}
