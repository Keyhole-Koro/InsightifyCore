package runner

import (
	llmclient "insightify/internal/llmClient"
	"insightify/internal/mcp"
	"insightify/internal/safeio"
)

type ArtifactAccess interface {
	Read(name string) ([]byte, error)
	Write(name string, content []byte) error
	Remove(name string) error
	List() ([]string, error)
}

// Runtime is the execution context required by runner and worker specs.
// Gateway ProjectRuntime/ExecutionRuntime and other runtimes can implement this.
type Runtime interface {
	GetOutDir() string
	GetRepoFS() *safeio.SafeFS
	Artifacts() ArtifactAccess
	GetResolver() SpecResolver
	GetMCP() *mcp.Registry
	GetModelSalt() string
	GetForceFrom() string
	GetDepsUsage() DepsUsageMode
	GetLLM() llmclient.LLMClient
}
