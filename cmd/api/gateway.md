# API Gateway Architecture

This document explains what each `gateway_*.go` file does in `cmd/api`, and how requests flow through the gateway.

## File Responsibilities

- `gateway_contract.go`
  - Compile-time interface conformance: `apiServer` implements `PipelineServiceHandler`.

- `gateway_state.go`
  - Shared in-memory gateway state.
  - `runStore`: `run_id -> event channel` for `WatchRun`.
  - `initRunStore`: `session_id -> initSession` for session and run context tracking.
  - `scheduleRunCleanup`: delayed cleanup of completed runs from `runStore`.

- `gateway_session_store.go`
  - Session persistence and helper accessors.
  - JSON persistence file: `tmp/init_sessions.json`.
  - Helpers: `getSession`, `putSession`, `updateSession`, `ensureSessionRunContext`.
  - Central place for session load/save and safe updates.

- `gateway_session.go`
  - Session utility functions.
  - `inferRepoName`: normalize repo name from URL.
  - `resolveSessionID` and `resolveSessionIDFromCookieHeader`: session resolution from request/cookie.

- `gateway_run_context.go`
  - Run execution environment creation.
  - `RunContext` definition and `NewRunContext`.
  - Builds LLM client, filesystem sandbox wrappers, MCP registry, and merged worker resolver.

- `gateway_run_execute.go`
  - Generic worker launch and execution pipeline.
  - `launchWorkerRun`: allocates run ID, marks session running, creates event channel.
  - `executeWorkerRun`: resolves worker, executes via runner, emits events.
  - `bridgeRunnerEvents`: maps internal runner events to API stream events.
  - `updateSessionFromResult`: writes back purpose/repo updates from `plan.BootstrapOut`.
  - `launchPlanPipelineRun`: convenience wrapper for `plan_pipeline`.

- `gateway_init.go`
  - `InitRun` endpoint implementation.
  - Resolves/creates session, ensures run context, persists session, starts bootstrap run, sets session cookie.

- `gateway_start.go`
  - `StartRun` endpoint implementation.
  - Handles:
    - `plan_pipeline` / `init_purpose` via `launchPlanPipelineRun`.
    - `test_pipeline` streaming demo path.
    - generic worker execution path with `runner.ExecuteWorkerWithResult`.

- `gateway_submit.go`
  - `SubmitRunInput` endpoint implementation.
  - Validates `session_id`, `run_id`, and `input`.
  - Ensures session run context.
  - Validates active run consistency.
  - Launches `plan_pipeline` continuation run.

- `gateway_watch.go`
  - Streaming endpoints:
    - `WatchRun` (Connect stream RPC)
    - `handleWatchSSE` (SSE fallback endpoint)
  - Reads from `runStore` event channel and streams until terminal event or disconnect.

## Core Runtime Model

Gateway runtime state is split into two maps:

- `initRunStore.sessions[session_id]`
  - User/repo/purpose data
  - `RunCtx`
  - run flags (`Running`, `ActiveRunID`)

- `runStore.runs[run_id]`
  - event channel for stream consumers (`WatchRun` / SSE)

The gateway orchestrates worker runs, while worker DAG execution is delegated to `internal/runner`.

## End-to-End Flows

### 1) Init Bootstrap Flow (`InitRun` -> bootstrap `plan_pipeline`)

```mermaid
sequenceDiagram
    autonumber
    participant FE as Frontend
    participant GW as API Gateway
    participant SS as initRunStore
    participant EX as run_execute
    participant RS as runStore
    participant WR as runner/workers

    FE->>GW: InitRun(user_id, repo_url)
    GW->>SS: load session store (once)
    GW->>SS: resolve cookie session or create new session
    GW->>GW: ensure RunContext (NewRunContext if missing)
    GW->>SS: persist session metadata
    GW->>EX: launchPlanPipelineRun(session_id, "", isBootstrap=true)
    EX->>SS: set Running=true, ActiveRunID=run_id
    EX->>RS: create event channel for run_id
    EX->>WR: executeWorkerRun(plan_pipeline)
    GW-->>FE: InitRunResponse(session_id, bootstrap_run_id) + Set-Cookie
```

### 2) Interactive Input Flow (`SubmitRunInput`)

```mermaid
sequenceDiagram
    autonumber
    participant FE as Frontend
    participant GW as API Gateway
    participant SS as initRunStore
    participant EX as run_execute
    participant WR as runner/workers

    FE->>GW: SubmitRunInput(session_id, run_id?, input)
    GW->>SS: getSession(session_id)
    alt session not found
        GW-->>FE: NotFound(session not found)
    else found
        GW->>GW: ensureSessionRunContext(session_id)
        GW->>SS: validate run_id == ActiveRunID (when provided)
        GW->>EX: launchPlanPipelineRun(session_id, input, false)
        EX->>WR: executeWorkerRun(plan_pipeline)
        GW-->>FE: SubmitRunInputResponse(next_run_id)
    end
```

### 3) Stream Consumption Flow (`WatchRun`)

```mermaid
sequenceDiagram
    autonumber
    participant FE as Frontend
    participant GW as API Gateway
    participant RS as runStore
    participant CH as run event channel

    FE->>GW: WatchRun(run_id)
    GW->>RS: lookup run_id
    alt run not found
        GW-->>FE: NotFound(run not found)
    else run found
        loop until terminal or disconnect
            CH-->>GW: WatchRunResponse event
            GW-->>FE: stream event
        end
    end
```

## Request Routing Summary

```mermaid
flowchart TD
    A[InitRun] --> B[Ensure/Create Session]
    B --> C[Ensure RunContext]
    C --> D[Launch Bootstrap plan_pipeline]
    D --> E[Return session_id + bootstrap_run_id]

    F[StartRun] --> G{pipeline_id}
    G -->|plan_pipeline/init_purpose| H[launchPlanPipelineRun]
    G -->|test_pipeline| I[Test Streaming Pipeline]
    G -->|other| J[Resolve worker by key]
    J --> K[Execute via runner]

    L[SubmitRunInput] --> M[Validate session/input]
    M --> N[Ensure RunContext]
    N --> O[Validate active run relation]
    O --> P[launchPlanPipelineRun]

    Q[WatchRun/SSE] --> R[Read runStore channel]
    R --> S[Stream LOG/PROGRESS/COMPLETE/ERROR]
```

## Notes and Constraints

- `StartRun` has two execution styles today:
  - Generic direct execution path inside `gateway_start.go`
  - Helper path via `gateway_run_execute.go` (`launchPlanPipelineRun`)
- `SubmitRunInput` currently routes to `plan_pipeline` by design.
- Session persistence is best-effort JSON persistence; run channels are in-memory only.
- `runStore` cleanup is delayed (`completedRunRetention`) to allow late watchers.
