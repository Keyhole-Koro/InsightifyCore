# Worker Update Guide

This document summarizes the internal runner pipeline and artifact relationships in InsightifyCore, plus what to update when adding a worker.

## Runner Pipeline & Artifacts (Internal)

### WorkerSpec → Artifact Flow

Each worker is registered as a `runner.WorkerSpec` with these fields:
- `Key`: logical worker ID (e.g., `code_roots`).
- `File`: logical artifact filename (e.g., `c0.json`).
- `Requires`: upstream worker keys (enforced).
- `BuildInput`: reads upstream artifacts via `Deps.Artifact`.
- `Run`: executes the pipeline and returns `WorkerOutput{RuntimeState, ClientView}`.
- `Fingerprint`: input hash for caching.
- `Strategy`: artifact persistence policy (`jsonStrategy` / `versionedStrategy`).

Execution path:
1. `ExecuteWorker` ensures all `Requires` exist (builds them if missing).
2. `BuildInput` reads required artifacts (strictly enforced).
3. Cache is checked by `Strategy.TryLoad` using `Fingerprint`.
4. `Run` executes if cache miss.
5. `Strategy.Save` persists the output artifact.

### Artifact Storage (OutDir)

Artifacts are written under `Env.OutDir` (CLI: `--out`, API: `artifacts/<repo>/<session>`).

`jsonStrategy`:
- Output: `OutDir/<worker_key>/output.json`
- Meta: `OutDir/<worker_key>/meta.json`
- Used by most workers (e.g., `code_imports`, `arch_design`, `infra_context`)

`versionedStrategy`:
- Output (versioned): `OutDir/<worker_key>_v1.json` (always reset to v1 per run)
- Latest pointer: `OutDir/<spec.File>` (e.g., `c0.json`, `c1.json`)
- Meta: `OutDir/<worker_key>.meta.json`
- Used by `code_roots` and `code_specs`

Artifact reads:
- `Deps.Artifact` / `runner.Artifact` use the registry’s `Strategy` to resolve paths.
- Fallback lookup uses `OutDir/<worker_key>.json` if a legacy file exists.

### Dependency Enforcement

- `Deps.Artifact(key, &out)` fails if `key` is not listed in `Requires`.
- Unused `Requires` are validated and can error/warn depending on `Env.DepsUsage`.

### Optional: Prompt/Streaming Outputs

- CLI can save LLM prompts to `artifacts/prompt/<worker>.txt` (see `runner.PromptSaver`).
- The API gateway can stream `runner.RunEvent` events for progress and LLM chunks.

## What to Update When Adding a Worker

When adding a new worker, update the following locations (as applicable):

1. **Artifacts & pipeline logic**: define input/output structs in `InsightifyCore/internal/artifact`, implement the pipeline runner in `InsightifyCore/internal/pipeline/<domain>`.
2. **Register the worker**: add a `WorkerSpec` in the appropriate registry. Locations: `InsightifyCore/internal/runner/code_registry.go` (codebase), `InsightifyCore/internal/runner/infra_registry.go` (external/infra), `InsightifyCore/internal/runner/architecture_registry.go` (architecture for CLI), `InsightifyCore/internal/runner/arch_registry.go` (architecture for API), `InsightifyCore/internal/runner/plan_registry.go` (planning). Set `Key`, `File`, `Requires`, `BuildInput`, `Run`, `Fingerprint`, `Strategy`.
3. **Include in registry merges**: CLI is `InsightifyCore/cmd/archflow/main.go` → `MergeRegistries(...)`, API is `InsightifyCore/cmd/api/run_context.go` → `MergeRegistries(...)`. If you introduce a brand‑new registry, add it to both.
4. **Expose CLI phase (if needed)**: update the `--phase` help and the unknown‑phase error list in `InsightifyCore/cmd/archflow/main.go`.
5. **Update visualization (if needed)**: add new registries to `InsightifyCore/internal/runner/viz.go` so the Mermaid graph includes the worker.
6. **Docs**: update this file with the new worker summary and dependencies.
