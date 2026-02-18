package runner

import (
	"insightify/internal/common/safeio"
	llmclient "insightify/internal/llm/client"
	"insightify/internal/mcp"
)

// Runtime is the execution context required by runner and worker specs.
// Gateway ProjectRuntime/ExecutionRuntime and other runtimes can implement this.
type Runtime interface {
	GetOutDir() string
	GetRepoFS() *safeio.SafeFS
	Artifacts() ArtifactStore
	GetResolver() SpecResolver
	GetMCP() *mcp.Registry
	GetModelSalt() string
	GetForceFrom() string
	GetDepsUsage() DepsUsageMode
	GetLLM() llmclient.LLMClient
}
