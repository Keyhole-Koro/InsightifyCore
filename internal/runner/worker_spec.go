package runner

import (
	"context"
)

// WorkerOutput bundles internal RuntimeState with an optional ClientView payload for the client.
type WorkerOutput struct {
	RuntimeState any
	ClientView   any
}

// WorkerSpec declares "what" a worker needs, not "how" the app calls it.
type WorkerSpec struct {
	Description string

	Key         string                                            // e.g. "m0"
	BuildInput  func(ctx context.Context, deps Deps) (any, error) // produce logical input
	Run         func(ctx context.Context, in any, runtime Runtime) (WorkerOutput, error)
	Fingerprint func(in any, runtime Runtime) string // stable hash for caching
	Downstream  []string                             // automatically computed
	Requires    []string
	Strategy    CacheStrategy // how to cache (json, versioned, none)
}

// CacheStrategy abstracts artifact persistence policies (json, versioned, …).
type CacheStrategy interface {
	// TryLoad returns (out, true) if cache hit and not forced.
	TryLoad(ctx context.Context, spec WorkerSpec, runtime Runtime, inputFP string) (WorkerOutput, bool)
	// Save persists result and metadata.
	Save(ctx context.Context, spec WorkerSpec, runtime Runtime, out WorkerOutput, inputFP string) error
	// Invalidate removes outputs/meta for this worker (used for downstream invalidation).
	Invalidate(ctx context.Context, spec WorkerSpec, runtime Runtime) error
}
