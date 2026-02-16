package runner

import (
	llmclient "insightify/internal/llmClient"
	"insightify/internal/mcp"
	"insightify/internal/safeio"
)

// Runtime is the execution context required by runner and worker specs.
// Gateway ProjectRuntime/ExecutionRuntime and other runtimes can implement this.
type Runtime interface {
	GetOutDir() string
	GetRepoFS() *safeio.SafeFS
	GetArtifactFS() *safeio.SafeFS
	GetResolver() SpecResolver
	GetMCP() *mcp.Registry
	GetModelSalt() string
	GetForceFrom() string
	GetDepsUsage() DepsUsageMode
	GetLLM() llmclient.LLMClient
}
