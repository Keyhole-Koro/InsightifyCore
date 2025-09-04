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

const bt = "`"

// X0 prompt — imports/includes only, plus normalization hints for later post-processing.
// This phase does NOT receive source snippets; it must produce RE2 patterns to detect module strings,
// and also provide language-agnostic guidance ("normalize_hints") describing how X1 should derive
// fields like folder_path[], file_name, file_ext, scope/package/subpath, alias, and kind.
const promptX0 = `You are an experienced software engineer working on a codebase.
Approach: Identify the relevant code → specify function and file names using exact notation → no guessing; if something is unknown, say it's unknown.

Role:
Generate lightweight Extractor Specs ONLY for import/include/use dependencies per extension present in the repository. These specs will be compiled into Go RE2 regex and used to build a static dependency graph. Do not create rules for calls, I/O, or annotations — imports/includes only.

Inputs (JSON):
{
  "project_hints": { "notes": "string (optional)" },
  "runtime_config_files": ["string"], // optional hints (e.g., tsconfig.json, webpack.config.js, pyproject.toml)
  "ext_report": [
    {
      "ext": ".ts",
      "count": 123,
      "sample_paths": ["src/app/app.module.ts", "src/foo/bar.ts"],
      "head_snippet": "first ~20 lines (may be absent)",
      "random_lines": ["one representative line", "another line (may be absent)"]
    }
    // ...multiple entries
  ],
  "existing_specs": []
}

Task:
For each relevant code extension, produce ExtractorSpec v1 with rules that capture only import-like statements:
- JS/TS: ` + "`" + `import ... from 'mod'` + "`" + `, ` + "`" + `import 'mod'` + "`" + `, ` + "`" + `require('mod')` + "`" + `, ` + "`" + `import('mod')` + "`" + `
- Go: ` + "`" + `import "pkg"` + "`" + `, ` + "`" + `import ("a"\n"b")` + "`" + `
- Python: ` + "`" + `import pkg.sub` + "`" + `, ` + "`" + `from pkg.sub import X` + "`" + `
- Rust: ` + "`" + `use crate::...;` + "`" + `, ` + "`" + `use std::...;` + "`" + `
- Java/Kotlin: ` + "`" + `import a.b.c;` + "`" + `
- Swift: ` + "`" + `import Module` + "`" + `
- C/C++: ` + "`" + `#include "local.h"` + "`" + `, ` + "`" + `#include <stdio.h>` + "`" + `
- Shell: ` + "`" + `source ./x.sh` + "`" + ` or ` + "`" + `. ./x.sh` + "`" + `
If an extension is not code or there is insufficient evidence, return an empty rules list and lower confidence.

Engine constraints (Go RE2):
- No lookbehind, no backreferences, no atomic groups.
- Use ^ / $ anchors where helpful. Use only capturing groups (...). Keep each regex <~200 chars.
- Avoid matching inside string literals or comments. Since lookbehind is not available, approximate by requiring a safe left boundary (?:^|[^"'A-Za-z0-9_]) before the keyword (e.g., import/require/use/include).
- Prefer explicit quoting groups like ['"] to reference string delimiters.

Normalization (IMPORTANT, for a later phase X1-parse):
Because this phase only detects module strings, include language-agnostic "normalize_hints" at the SPEC level to guide post-processing. Do NOT attempt to split in the regex. Provide rules so X1 can derive:
- kind: "path" | "package" | "system_header" | "module"
- folder_path: string[]   // for path kind
- file_name: string       // for path kind
- file_ext: string        // for path kind (may be empty)
- scope, package, subpath // for npm-like module names
- alias: string           // if runtime configs suggest alias mapping (e.g., tsconfig paths)
Also include a few short normalize examples.

Suggested normalization heuristics (encode them under normalize_hints):
- Classify:
  * If raw starts with ".", "..", "/", or drive-letter like "C:\", => kind="path".
  * If raw matches "^@[^/]+/[^/]+(?:/.*)?$", => kind="package" (scoped npm).
  * If raw matches "^[^@./][^/]*(?:/.*)?$", => kind="package" (unscoped).
  * C/C++ angle includes like "<stdio.h>" => kind="system_header".
  * Otherwise => kind="module".
- Path split (for kind="path" or C/C++ quotes includes):
  * Split by "/" or "\"; last segment → file_name (+ file_ext), preceding → folder_path[].
- NPM split (for kind="package"):
  * scoped: "@scope/pkg/sub/a" → scope="@scope", package="pkg", subpath="sub/a".
  * unscoped: "lodash/fp" → scope="", package="lodash", subpath="fp".
- Alias:
  * If runtime_config_files mention tsconfig paths or bundler aliases, set alias to the matching prefix (e.g., "~", "@src", "#app").
  * Otherwise alias="".

Output: STRICT JSON ONLY (no comments), schema:
{
  "version": 1,
  "specs": [
    {
      "ext": ".ts",
      "language": "TypeScript",
      "comment_styles": { "line": ["//"], "block": [{"start":"/*","end":"*/"}] },
      "string_delims": ["\"", "'",` + bt + `],
      "rules": [
        {
          "id": "import_ts_any",
          "kind": "bind",
          "style": "regex",
          "pattern": "(?:^|[^A-Za-z0-9_.$])(?:(?:import|export)\\s+.*?\\s*from|import|require)\\s*(?:\\(\\s*)?['\"]([^'\"]+)['\"]",
          "captures": { "module": 1 },
          "attrs": { "surface": "module" },
          "tests": {
            "pos": [
              "import { Foo } from 'bar';",
              "import 'side-effect';",
              "const m = require('mod');",
              "await import('dyn/mod');",
              "export { Bar } from 'foo';",
              "export * from '@org/pkg';"
            ],
            "neg": [
              "// import 'fake'",
              "const s = \"import from 'str'\"",
              "function my_import() {}",
              "const requires = true;",
              "new Exporter('a.js');",
              "export default function() {}"
            ]
          }
        }
      ],
      "normalize_hints": {
        "classify": [
          { "if": "^(?:\\.|\\.\\.|/|[A-Za-z]:\\\\)", "kind": "path" },
          { "if": "^<[^>]+>$", "kind": "system_header" },
          { "if": "^@[^/]+/[^/]+(?:/.*)?$", "kind": "package", "scoped": true },
          { "if": "^[^@./][^/]*(?:/.*)?$", "kind": "package", "scoped": false }
        ],
        "path_split": { "sep": "[/\\\\]", "emit": ["folder_path[]","file_name","file_ext"] },
        "npm_split": { "emit": ["scope","package","subpath"] },
        "alias_hints": ["^~\\/","^@src\\/","^#app\\/"],
        "examples": [
          { "raw": "@org/pkg/utils/a", "kind": "package", "scope": "@org", "package": "pkg", "subpath": "utils/a", "alias": "" },
          { "raw": "./utils/a.ts", "kind": "path", "folder_path": ["utils"], "file_name": "a", "file_ext": ".ts" },
          { "raw": "<stdio.h>", "kind": "system_header", "file_name": "stdio.h", "file_ext": ".h" }
        ]
      },
      "notes": ["imports/includes only; no calls or I/O surfaces"],
      "confidence": 0.9
    }
    // one spec per present code extension (.go, .py, .rs, .c/.h, .cpp/.hpp, .java, .kt, .swift, .sh, etc.)
  ]
}

Requirements:
- Only 'bind' rules. Do not output invoke/io/annotate/file_head rules.
- For each rule, include >= 6 positive and >= 6 negative single-line tests.
- Prefer a single rule per family where possible (e.g., one JS rule that covers import/require/dynamic import). If you add more, keep total ≤ 3 per extension.
- Tailor to ext_report when possible; otherwise keep generic-safe.
- If unknown/ambiguous, return empty rules and lower confidence.
- Return STRICT JSON only; no comments or trailing commas.
`

// X0 generates ExtractorSpec v1 rules per extension based on an ext report.
type X0 struct{ LLM llm.LLMClient }

// Run asks the model to produce specs, validates them against provided tests,
// and retries once with additional context when available (e.g., runtime configs).
func (p *X0) Run(ctx context.Context, in t.X0In) (t.X0Out, error) {
	// First attempt
	out, raw, err := p.generate(ctx, in)
	if err == nil {
		if verr := validateX0(out); verr == nil {
			return out, nil
		}
		// If validation failed, retry once with a regeneration hint.
	}

	// Prepare a single retry with a regen hint.
	hint := map[string]any{
		"reason": "previous attempt failed tests; tighten regex and fix captures; ensure >=6 pos/neg tests per rule",
	}
	_ = raw // keep raw for local debugging if needed

	input := map[string]any{
		"ext_report":           in.ExtReport,
		"existing_specs":       in.ExistingSpecs,
		"runtime_config_files": in.RuntimeConfigFiles,
		"regen_hint":           hint,
	}
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
		"ext_report":           in.ExtReport,
		"existing_specs":       in.ExistingSpecs,
		"runtime_config_files": in.RuntimeConfigFiles,
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
			// All positives must match somewhere.
			for _, s := range rule.Tests.Pos {
				if !re.MatchString(s) {
					return fmt.Errorf("rule %s for %s failed positive: %q", rule.ID, spec.Ext, s)
				}
			}
			// No negative should match (after crude string/comment stripping).
			for _, s := range rule.Tests.Neg {
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
// from a single line according to the spec's delimiters, returning a sanitized line.
func stripStringsAndLineComments(s string, delims []string, cs *t.CommentStyle) string {
	// Strip line comments.
	if cs != nil && len(cs.Line) > 0 {
		for _, tok := range cs.Line {
			if tok == "" {
				continue
			}
			if i := indexOf(s, tok); i >= 0 {
				s = s[:i]
			}
		}
	}
	if len(delims) == 0 {
		return s
	}
	// Very simple string stripper: toggles when meeting a delimiter; does not handle escapes fully.
	in := rune(0)
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if in == 0 {
			// Not inside a string.
			if isAnyQuote(r, delims) {
				in = r
				out = append(out, ' ')
				continue
			}
			out = append(out, r)
		} else {
			// Inside a string; look for closing same quote.
			if r == in {
				in = 0
				out = append(out, ' ')
			} else {
				out = append(out, ' ')
			}
		}
	}
	return string(out)
}

func isAnyQuote(r rune, delims []string) bool {
	for _, d := range delims {
		if len(d) == 1 && rune(d[0]) == r {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	return strings.Index(s, sub)
}
