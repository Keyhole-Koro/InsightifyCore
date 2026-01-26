# MCP (in-process tools)

Thin wrapper to run MCP-style tools **in-process** inside Insightify Core.
No IPC; tools are invoked directly via `Registry.Call()`.

## Overview

- `Registry` : registers and calls tools
- `Tool`     : minimal interface with `Spec()` and `Call()`
- `Host`     : DI container for repo/artifact access

## Usage (register)

```go
host := mcp.Host{
  RepoRoot: repoRoot,
  RepoFS: repoFS,
  ArtifactFS: artifactFS,
}
reg := mcp.NewRegistry()
mcp.RegisterDefaultTools(reg, host)
```

## Usage (call)

```go
out, err := reg.Call(ctx, "scan.list", inputJSON)
```

## Default tools

- `scan.list`       : list files under repo
- `fs.read`         : read repo files
- `wordidx.search`  : search via word index
- `snippet.collect` : collect related snippets from C4 artifact
- `delta.diff`      : compute JSON delta between before/after

## Tool contract

Schema lives at `schema/mcp/insightify.tools.yaml`.
Treat this YAML as the tool contract; implementation is in Go.
