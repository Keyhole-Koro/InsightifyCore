# Cache/Repository Unification (Breaking Change)

## Goal

Unify implementation rules across `artifact`, `project`, `ui`, and `uiworkspace`, and enforce:

- read-through cache
- write-through cache
- TTL + LRU cache policy
- on cache miss, read from origin
- on write, update/invalidate cache only after origin write succeeds

Backward compatibility is intentionally not preserved.

## Layer Rules

- `internal/gateway/repository/*`: origin repository contracts and origin implementations
- `internal/cache/*`: reusable cache backends
  - `internal/cache/memory`: memory cache primitives (LRU+TTL)
  - `internal/cache/disk`: local disk-backed cache primitives

Repository packages must not embed ad-hoc cache state.  
Caching behavior is provided via explicit `CachedStore` decorators.

## Unified Pattern

Use the same pattern in each domain:

1. Origin store (`PostgresStore`, `S3Store`, etc.)
2. Cached store (`NewCachedStore(origin, cfg)`)
3. App composition injects the cached store (not the origin directly)

## Current Unified Components

- Artifact
  - Origin: `internal/gateway/repository/artifact/{postgres_store,s3_store}.go`
  - Cache decorator: `internal/cache/artifact/cached_store.go`
  - Memory origin/fallback: `internal/cache/artifact/memory_store.go`
  - Disk origin/fallback: `internal/cache/artifact/disk_store.go`

- Project
  - Origin: `internal/gateway/repository/project/postgres_store.go`
  - Cache decorator: `internal/cache/project/cached_store.go`
  - Memory origin/fallback: `internal/cache/project/memory_store.go`
  - Disk origin/fallback: `internal/cache/project/disk_store.go`

- UI
  - Origin: `internal/gateway/repository/ui/postgres_store.go`
  - Cache decorator: `internal/cache/ui/cached_store.go`
  - Memory origin/fallback: `internal/cache/ui/memory_store.go`
  - Disk origin/fallback: `internal/cache/ui/disk_store.go`

- UIWorkspace
  - Origin: `internal/gateway/repository/uiworkspace/postgres_store.go`
  - Cache decorator: `internal/cache/uiworkspace/cached_store.go`
  - Memory origin/fallback: `internal/cache/uiworkspace/memory_store.go`
  - Disk origin/fallback: `internal/cache/uiworkspace/disk_store.go`

## Cache Package Layout

Domain cache implementations now live under `internal/cache/<domain>`.

- `memory_store.go`: in-memory origin/fallback
- `disk_store.go`: local disk-backed origin/fallback
- `cached_store.go`: cache orchestration around repository interfaces

This keeps the pattern consistent across domains.

## Context Contract

Repository interfaces for `project/ui/uiworkspace/artifact` accept `context.Context`.
Avoid hard-coded `context.Background()` for I/O paths; propagate caller `ctx`.

## Cache Policy

Shared memory cache implementation uses `internal/cache/memory/lru_ttl.go`.

- TTL expiration is treated as miss
- LRU eviction enforces capacity limits
- explicit invalidation via `Clear/Delete`

## Metrics

Artifact cache exposes runtime counters via:

- `internal/cache/artifact/cached_store.go`
  - `CachedStore.Metrics() MetricsSnapshot`

Counters currently include hit/miss and origin read/write success/failure totals.

## App Wiring

`internal/gateway/app/app.go` and `internal/gateway/app/stores.go`
must inject repositories through `NewCachedStore` instead of using origins directly.

This guarantees that all consumers get consistent cache behavior
through repository interfaces without extra wiring.
