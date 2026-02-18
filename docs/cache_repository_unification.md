# Cache/Repository Unification (Current Implementation)

## Scope

This document describes the current cache/repository design for:

- `artifact`
- `project`
- `ui`
- `uiworkspace`
- runner artifact I/O (`runner.ArtifactStore`)

The goal is a consistent decorator-based caching approach while keeping worker runtime artifact storage separate from gateway repository caching.

## Layer Boundaries

- `internal/gateway/repository/*`
  - Origin repository contracts and DB/S3-backed implementations.
- `internal/cache/*`
  - Cache decorators and local fallback stores.
  - Shared primitives:
    - `internal/cache/memory/lru_ttl.go`
    - `internal/cache/disk/lru_ttl_store.go`
- `internal/runner/*`
  - Worker execution abstractions (`Runtime`, `ArtifactStore`, cache strategies).
- `internal/workerruntime/*`
  - Runtime composition and default runner artifact store.

Caching behavior is implemented in `CachedStore` decorators, not as ad-hoc state inside origin repositories.

## Unified Domain Pattern

For each domain, the active pattern is:

1. Build an origin store (`PostgresStore`, `S3Store`, or in-memory/disk fallback).
2. Wrap it with `NewCachedStore(...)`.
3. Inject the cached store into services (do not inject origin directly).

## Domain Status

### Artifact

- Origin examples:
  - `internal/gateway/repository/artifact/postgres_store.go`
  - `internal/gateway/repository/artifact/s3_store.go`
- Cache decorator:
  - `internal/cache/artifact/cached_store.go`
- Local fallback origins:
  - `internal/cache/artifact/memory_store.go`
  - `internal/cache/artifact/disk_store.go`

Behavior:

- Read-through for `Get`, `List`, `GetURL`.
- Write-through for `Put`.
- On successful `Put`, blob cache is updated and list/URL caches are invalidated.
- Metrics are available via `CachedStore.Metrics()`.

### Project

- Origin:
  - `internal/gateway/repository/project/postgres_store.go`
- Cache decorator:
  - `internal/cache/project/cached_store.go`
- Local fallback origins:
  - `internal/cache/project/memory_store.go`
  - `internal/cache/project/disk_store.go`

Notes:

- `NewCachedStore(origin Repository, meta ArtifactRepository, cfg CacheConfig)` uses two dependencies:
  - `origin` for project state methods.
  - `meta` for project artifact metadata methods (`AddArtifact`, `ListArtifacts`).
- Read-through/write-through is applied across project state and artifact metadata cache entries.

### UI

- Origin:
  - `internal/gateway/repository/ui/postgres_store.go`
- Cache decorator:
  - `internal/cache/ui/cached_store.go`
- Local fallback origins:
  - `internal/cache/ui/memory_store.go`
  - `internal/cache/ui/disk_store.go`

Behavior:

- `GetDocument` is read-through.
- `ApplyOps` writes to origin first, then updates/invalidate cache depending on returned document.
- Proto documents are cloned on read/write boundaries to avoid shared mutable objects.

### UIWorkspace

- Origin:
  - `internal/gateway/repository/uiworkspace/postgres_store.go`
- Cache decorator:
  - `internal/cache/uiworkspace/cached_store.go`
- Local fallback origins:
  - `internal/cache/uiworkspace/memory_store.go`
  - `internal/cache/uiworkspace/disk_store.go`

Behavior:

- Read-through for workspace/tab fetch APIs.
- Write-through + targeted invalidation for tab/workspace mutations (`CreateTab`, `SelectTab`, `UpdateTabRun`).

## Runner Artifact I/O (Separated Concern)

Runner artifact I/O is intentionally separate from gateway repository caches.

- Contract:
  - `internal/runner/artifact_store.go`
  - `type ArtifactStore interface { Read/Write/Remove/List(context.Context, name) }`
- Runtime injection:
  - `internal/workerruntime/project_runtime.go`
  - `ExecutionOptions.ArtifactStore`
- Default implementation:
  - `internal/workerruntime/artifactfs/file_store.go` (`artifactfs.NewFileStore(outDir)`)

This keeps worker execution decoupled from gateway artifact repository implementations.

## Cache Primitives and Policy

### Memory LRU+TTL

`internal/cache/memory/lru_ttl.go` provides:

- TTL expiration (expired entries behave as cache misses).
- LRU eviction.
- Size-aware admission/eviction using `maxBytes` where configured.
- Explicit invalidation (`Delete`, `Clear`).

### Disk LRU+TTL

`internal/cache/disk/lru_ttl_store.go` provides:

- File-backed values.
- Persistent metadata/index (`size`, expiration, access recency).
- Startup cleanup for expired/missing entries.
- LRU eviction for `MaxEntries`/`MaxBytes`.

## App Wiring (Current)

- `internal/gateway/app/app.go`
  - Manually wires Postgres/S3 origins and wraps them with cache decorators.
- `internal/gateway/app/stores.go`
  - Provides environment-dependent wiring (Postgres or in-memory fallback), then applies cache decorators.
  - Artifact origin is selected via `chooseArtifactStore(...)`, then wrapped by `artifactcache.NewCachedStore(...)`.

## Known Gaps / Follow-ups

- `internal/runner/deps.go` currently reads artifacts with `context.Background()` inside `Deps.Artifact(...)` instead of forwarding caller context.
  - If strict context propagation is required, this should be changed to pass execution/build context through `Deps`.

- Not every domain currently exposes cache metrics.
  - Runtime metrics API exists today only for artifact cache (`artifact.CachedStore.Metrics()`).
