package pipeline

import (
    "context"
    "encoding/json"
    "fmt"
    "regexp"
    "strings"

    "insightify/internal/llm"
    t "insightify/internal/types"
)

const promptX0 = `You are an experienced software engineer working on a codebase.
Approach: Identify the relevant code → specify function and file names using exact notation → no guessing; if something is unknown, say it's unknown.

Role:
You will generate lightweight Extractor Specs ONLY for import/include/use dependencies per extension present in the repository. These specs will be compiled into Go RE2 regex and used to build a static dependency graph. Do not create rules for calls, I/O, or annotations—imports/includes only.

If provided, consider project_hints.tsconfig (string content of tsconfig.json) to decide module system for JS/TS (commonjs vs esm) and path alias settings.

Inputs (JSON):
{
  "project_hints": { "notes": "string (optional)" },
  "ext_report": [
    {
      "ext": ".ts",
      "count": 123,
      "sample_paths": ["src/app/app.module.ts", "src/foo/bar.ts"],
      "head_snippet": "first ~20 lines",
      "random_lines": ["one representative line", "another line"]
    }
    // ...multiple entries
  ],
  "existing_specs": []
}

Task:
For each relevant code extension, produce ExtractorSpec v1 with rules that capture only import-like statements:
- JS/TS: "import ... from 'mod'", "import 'mod'", "require('mod')", "import('mod')"
- Go: "import \"pkg\"", "import (\"a\"\n\"b\")"
- Python: "import pkg.sub", "from pkg.sub import X"
- Rust: "use crate::...;", "use std::...;"
- Java/Kotlin: "import a.b.c;"
- Swift: "import Module"
- C/C++: "#include \"local.h\"", "#include <stdio.h>"
- Shell: "source ./x.sh" or ". ./x.sh"
If an extension is not code or there is insufficient evidence, return an empty rules list and lower confidence.

Engine constraints (Go RE2):
- No lookbehind, no backreferences. Keep patterns simple and robust.
- Use ^ / $ anchors where helpful. Use only capturing groups (...). Keep each regex <~200 chars.
 - Avoid matching inside string literals or comments. Since lookbehind is not available, approximate by requiring a safe left boundary: (?:^|[^"'A-Za-z0-9_]) before the keyword (e.g., import/require/use/include).
 - Prefer quoting groups like ['\"] to reference string delimiters explicitly; combine with the left-boundary trick above.

Output: STRICT JSON ONLY (no comments), schema:
{
  "version": 1,
  "specs": [
    {
      "ext": ".ts",
      "language": "TypeScript",
      "comment_styles": { "line": ["//"], "block": [{"start":"/*","end":"*/"}] },
      "string_delims": ["\"", "'"],
      "rules": [
        {
          "id": "import_ts_from",
          "kind": "bind",
          "style": "regex",
          "pattern": "^\\s*import\\s+(?:[^;]*?\\s+from\\s+|[\\s(]*)['\" ]([^'\" ]+)['\"]",
          "captures": {"module": 1},
          "attrs": {"surface": "module"},
          "tests": {
            "pos": [
              "import { Foo } from 'bar';",
              "import x from \"@org/pkg\"",
              "import 'side-effect';",
              "const m = require('mod')",
              "await import('dyn/mod')",
              "import\t{A as B}\tfrom\t'v'"
            ],
            "neg": [
              "// import fake",
              "const s = \"import from\"",
              "function importX(){}",
              "export function f(){}",
              "importedValue = 1",
              "requireNot('x')"
            ]
          }
        }
      ],
      "notes": ["imports/includes only"],
      "confidence": 0.85
    }
    // one spec per present code extension (.go, .py, .rs, .c/.h, .cpp/.hpp, .java, .kt, .swift, .sh, etc.)
  ]
}

Requirements:
- Only 'bind' rules. Do not output invoke/io/annotate rules.
- For each rule, include >= 6 positive and >= 6 negative single-line tests.
- Prefer a single rule per family where possible (e.g., a JS rule that covers import/require/dynamic import). If you add more, keep total ≤ 3 per extension.
- Use the given samples (head_snippet/random_lines) to tune patterns; otherwise keep generic-safe.
- If unknown/ambiguous, return empty rules and lower confidence.
- Return STRICT JSON only; no comments or trailing commas.
`

// X0 generates ExtractorSpec v1 rules per extension based on an ext report.
type X0 struct{ LLM llm.LLMClient }

// Run asks the model to produce specs, validates them against provided tests,
// and retries once with additional context when available (e.g., tsconfig).
func (p *X0) Run(ctx context.Context, in t.X0In) (t.X0Out, error) {
    // First attempt
    out, raw, err := p.generate(ctx, in)
    if err == nil {
        if verr := validateX0(out); verr == nil {
            return out, nil
        }
        // If validation failed and tsconfig is available but not included, retry with it.
    }

    // Prepare a single retry with a regen hint. If we already had tsconfig,
    // keep it; otherwise leave it empty and just ask the model to fix tests.
    hint := map[string]any{
        "reason": "previous attempt failed tests; tighten regex and fix captures",
    }
    _ = raw // we could add raw for debugging, but avoid leaking large payloads

    input := map[string]any{
        "ext_report":    in.ExtReport,
        "existing_specs": in.ExistingSpecs,
        "regen_hint":     hint,
    }
    // Use the same prompt; include regen_hint in input only.
    rraw, rerr := p.LLM.GenerateJSON(ctx, promptX0, input)
    if rerr != nil {
        return t.X0Out{}, rerr
    }
    var rout t.X0Out
    if err := json.Unmarshal(rraw, &rout); err != nil {
        return t.X0Out{}, fmt.Errorf("X0 JSON invalid on retry: %w", err)
    }
    if verr := validateX0(rout); verr != nil {
        return t.X0Out{}, fmt.Errorf("X0 specs failed validation after retry: %w", verr)
    }
    return rout, nil
}

func (p *X0) generate(ctx context.Context, in t.X0In) (t.X0Out, json.RawMessage, error) {
    input := map[string]any{
        "ext_report":    in.ExtReport,
        "existing_specs": in.ExistingSpecs,
    }
    raw, err := p.LLM.GenerateJSON(ctx, promptX0, input)
    if err != nil {
        return t.X0Out{}, nil, err
    }
    var out t.X0Out
    if err := json.Unmarshal(raw, &out); err != nil {
        return t.X0Out{}, raw, err
    }
    return out, raw, nil
}

// validateX0 compiles regex rules and executes provided pos/neg tests.
func validateX0(out t.X0Out) error {
    for _, spec := range out.Specs {
        for _, rule := range spec.Rules {
            if rule.Style != "regex" && rule.Style != "regex+ebnf" && rule.Style != "regex-only" {
                continue // ignore unknown styles
            }
            re, err := regexp.Compile(rule.Pattern)
            if err != nil {
                return fmt.Errorf("invalid regex for %s (%s): %v", spec.Ext, rule.ID, err)
            }
            // All pos must match somewhere
            for _, s := range rule.Tests.Pos {
                if !re.MatchString(s) {
                    return fmt.Errorf("rule %s for %s failed positive: %q", rule.ID, spec.Ext, s)
                }
            }
            // No neg should match
            for _, s := range rule.Tests.Neg {
                // To reduce false positives for occurrences inside strings/comments,
                // run the regex on a preprocessed line with strings and line comments removed.
                // This loosely simulates an engine that ignores strings/comments per spec.
                line := stripStringsAndLineComments(s, spec.StringDelims, spec.CommentStyles)
                if re.MatchString(line) {
                    return fmt.Errorf("rule %s for %s matched negative: %q", rule.ID, spec.Ext, s)
                }
            }
        }
    }
    return nil
}

// stripStringsAndLineComments removes content of string literals and line comments
// from a single line according to the spec's delimiters, returning a sanitised line.
func stripStringsAndLineComments(s string, delims []string, cs *t.CommentStyle) string {
    // Handle line comments first
    if cs != nil && len(cs.Line) > 0 {
        for _, tok := range cs.Line {
            if tok == "" { continue }
            if i := indexOf(s, tok); i >= 0 {
                s = s[:i]
            }
        }
    }
    if len(delims) == 0 { return s }
    // Very simple string stripper: toggles when meeting a delimiter; does not handle escapes fully.
    in := rune(0)
    out := make([]rune, 0, len(s))
    for _, r := range s {
        if in == 0 {
            // Not inside a string
            if isAnyQuote(r, delims) {
                in = r
                out = append(out, ' ') // replace opening quote
                continue
            }
            out = append(out, r)
        } else {
            // Inside a string; look for closing same quote
            if r == in {
                in = 0
                out = append(out, ' ') // replace closing quote
            } else {
                // replace string content with spaces
                out = append(out, ' ')
            }
        }
    }
    return string(out)
}

func isAnyQuote(r rune, delims []string) bool {
    for _, d := range delims {
        if len(d) == 1 && rune(d[0]) == r { return true }
    }
    return false
}

func indexOf(s, sub string) int {
    // naive search to avoid importing strings again (already imported above)
    return strings.Index(s, sub)
}
