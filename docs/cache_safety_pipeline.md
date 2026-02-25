# Cache Safety Pipeline

This document defines the cache consistency contract for UI restore.

## Source of Truth

- Canonical persisted state:
  - `workspace_tabs` (tab -> run binding)
  - `user_interactions` (run -> UI document nodes)
- Non-canonical cache:
  - Browser `localStorage` metadata (`insightify.ui_doc_meta.*`)
  - In-process LRU caches in Core (`internal/cache/ui*`)

`localStorage` is an optimization layer only. It must never be treated as authoritative.

## Restore Contract

Frontend always restores node document from server response.
Browser cache stores metadata only (`run_id`, `document_hash`, `saved_at`) for diagnostics.

## Operational Guardrails

1. `DATABASE_URL` is required and must be shared by all local run/test commands.
2. Core startup logs DB identity (`database`, `server_addr`, `server_port`, `user`) for quick mismatch detection.
3. Inspection commands print DB fingerprint before querying tables.
4. Integration test verifies end-to-end persistence and restore across process restart.

## Verification Commands

```bash
make verify-cache-pipeline
make list-uiworkspace-nodes
make list-tab-nodes TAB_ID=tab-...
```
