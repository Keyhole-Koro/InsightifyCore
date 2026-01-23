# LLM Tool Loop (internal/llmtool)

Reusable component that lets an LLM iterate by emitting **tool calls**.
Phases call `ToolLoop.Run` to reuse the tool → result → re-prompt loop.

## Minimal usage

```go
loop := &llmtool.ToolLoop{
  LLM:      env.LLM,
  Tools:    env.MCP,       // ToolProvider
  MaxIters: 5,
  Allowed:  []string{"scan.list", "fs.read"}, // empty = allow all
}

finalJSON, state, err := loop.Run(ctx, input, llmtool.DefaultPromptBuilder(basePrompt))
```

## ToolLoop.Run

```go
func (l *ToolLoop) Run(
  ctx context.Context,
  input any,
  build PromptBuilder,
) (final json.RawMessage, state *ToolState, err error)
```

### ToolState

- `Input` : phase input snapshot
- `Iterations` : iteration count
- `ToolResults` : tool call history

## LLM output format

```json
{ "action": "tool",  "tool_name": "scan.list", "tool_input": { ... } }
{ "action": "final", "final": { ... } }
```

`final` should be the phase output JSON as-is.

## PromptBuilder

`DefaultPromptBuilder(basePrompt)` injects:

- `[TOOLS]` : ToolSpec JSON
- `[TOOL_RESULTS]` : tool output log so far

If you need a custom prompt layout, implement `PromptBuilder`.

## Errors

- `ErrMaxIterations` : reached maxIters
- `ErrToolNotAllowed` : tool not in Allowed
- `ErrUnknownAction` : unknown action
