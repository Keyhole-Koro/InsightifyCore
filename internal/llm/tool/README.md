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

## StructuredPromptBuilder

For most phases, prefer the structured builder:

```go
spec := llmtool.StructuredPromptSpec{
  Purpose:      "Summarize the repository.",
  Background:   "Phase M0 - overview.",
  OutputFormat: "JSON only.",
  Language:     "English",
  OutputFields: []llmtool.PromptField{
    {Name: "summary", Type: "string", Required: true, Description: "Short summary."},
    {Name: "risks", Type: "[]string"},
  },
  Constraints: []string{"No markdown."},
  Rules:       []string{"Be concise."},
  Assumptions: []string{"If unsure, return empty strings."},
}

finalJSON, state, err := loop.Run(ctx, input, llmtool.StructuredPromptBuilder(spec))
```

The builder renders these sections:
- `PURPOSE`
- `BACKGROUND`
- `INPUT` (JSON payload)
- `OUTPUT` (simple field list)
- `CONSTRAINTS`
- `RULES`
- `ASSUMPTIONS`
- `OUTPUT_FORMAT`
- `LANGUAGE`
- `TOOLS`
- `MCP_RESULTS`

## Presets

You can prepend shared constraints/rules via presets:

```go
spec := llmtool.ApplyPresets(
  llmtool.StructuredPromptSpec{ /* ... */ },
  llmtool.PresetStrictJSON(),
  llmtool.PresetNoInvent(),
  llmtool.PresetCautious(),
)
```

## Output fields from struct

You can derive `OutputFields` from a Go struct:

```go
fields := llmtool.MustFieldsFromStruct(ml.M1Out{})
spec := llmtool.StructuredPromptSpec{
  Purpose: "…",
  OutputFields: fields,
}
```

Tags:
- `prompt_desc:"..."` description
- `prompt_type:"Type"` override type string
- `prompt:"optional"` or `prompt:"required"`
- `prompt:"-"` to skip a field

## Errors

- `ErrMaxIterations` : reached maxIters
- `ErrToolNotAllowed` : tool not in Allowed
- `ErrUnknownAction` : unknown action
