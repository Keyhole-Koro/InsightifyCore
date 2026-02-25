# Act Contract (Core/Web Shared)

This document defines the canonical `act` vocabulary shared by Core and Web.

## Node Type

- `UI_NODE_TYPE_ACT` is the only new node type for the act-centric flow.
- Suggest/search/run outputs are represented as timeline events in the same act node.

## Act Status

- `UI_ACT_STATUS_IDLE`
- `UI_ACT_STATUS_PLANNING`
- `UI_ACT_STATUS_SUGGESTING`
- `UI_ACT_STATUS_SEARCHING`
- `UI_ACT_STATUS_RUNNING_WORKER`
- `UI_ACT_STATUS_NEEDS_USER_ACTION`
- `UI_ACT_STATUS_DONE`
- `UI_ACT_STATUS_FAILED`

## Mode

`mode` is a string used for lightweight UI/render control:

- `planning`
- `suggest`
- `search`
- `run_worker`
- `needs_user_action`
- `done`
- `failed`

## Timeline Event Kind

Recommended event kinds:

- `user_input`
- `plan`
- `suggestion`
- `search_result`
- `worker_start`
- `worker_output`
- `worker_error`
- `system_note`

## Actor Policy (Node Creation)

`CreateNodeInTab` allows only:

- `act`
- `worker`
- `system`

Any other actor is denied.
