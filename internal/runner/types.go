package runner

import (
	"context"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/mcp"
	"insightify/internal/safeio"
	"insightify/internal/utils"
	"insightify/internal/wordidx"
)

// Env is the shared environment passed to builders/workers.
type Env struct {
	Repo        string
	RepoRoot    string
	SourcePaths []string
	OutDir      string
	MaxNext     int
	RepoFS      *safeio.SafeFS
	ArtifactFS  *safeio.SafeFS
	Resolver    SpecResolver

	MCP     *mcp.Registry
	MCPHost mcp.Host

	ModelSalt string
	ForceFrom string
	DepsUsage DepsUsageMode

	LLM    llmclient.LLMClient
	UIDGen *utils.UIDGenerator

	WordIndexer wordidx.AggIndex

	Index  []artifact.FileIndexEntry
	MDDocs []artifact.MDDoc
}

// WorkerOutput bundles internal RuntimeState with an optional ClientView payload for the client.
type WorkerOutput struct {
	RuntimeState any
	ClientView   any
}

// WorkerSpec declares "what" a worker needs, not "how" the app calls it.
type WorkerSpec struct {
	Description string // ログやエラーメッセージ用の最小限の説明

	Key         string                                            // e.g. "m0"
	BuildInput  func(ctx context.Context, deps Deps) (any, error) // produce logical input
	Run         func(ctx context.Context, in any, env *Env) (WorkerOutput, error)
	Fingerprint func(in any, env *Env) string // stable hash for caching
	Downstream  []string                      // automatically computed
	Requires    []string
	Strategy    CacheStrategy // how to cache (json, versioned, none)
	// LLMLevel is required. Role/provider/model are optional hints.
	LLMRole     llm.ModelRole
	LLMLevel    llm.ModelLevel
	LLMProvider string
	LLMModel    string
}

// CacheStrategy abstracts artifact persistence policies (json, versioned, …).
type CacheStrategy interface {
	// TryLoad returns (out, true) if cache hit and not forced.
	TryLoad(ctx context.Context, spec WorkerSpec, env *Env, inputFP string) (WorkerOutput, bool)
	// Save persists result and metadata.
	Save(ctx context.Context, spec WorkerSpec, env *Env, out WorkerOutput, inputFP string) error
	// Invalidate removes outputs/meta for this worker (used for downstream invalidation).
	Invalidate(ctx context.Context, spec WorkerSpec, env *Env) error
}
