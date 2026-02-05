package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// PromptSaver implements llm.PromptHook to persist prompts & raw responses.
type PromptSaver struct{ Dir string }

// Before writes prompt and input JSON to artifacts/prompt/<worker>.txt
func (p *PromptSaver) Before(ctx context.Context, worker, prompt string, input any) {
	if worker == "" {
		worker = "unknown"
	}
	_ = os.MkdirAll(filepath.Join(p.Dir, "prompt"), 0o755)
	path := filepath.Join(p.Dir, "prompt", worker+".txt")

	var buf bytes.Buffer
	buf.WriteString("==== ")
	buf.WriteString(time.Now().Format(time.RFC3339))
	buf.WriteString(" ====\n")
	buf.WriteString(prompt)
	buf.WriteString("\n\n[INPUT JSON]\n")
	jb, _ := json.MarshalIndent(input, "", "  ")
	buf.Write(jb)
	buf.WriteString("\n\n")

	f, _ := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if f != nil {
		_, _ = f.Write(buf.Bytes())
		_ = f.Close()
	}
}

// After writes model raw response (or error) and also dumps <worker>.raw.json
func (p *PromptSaver) After(ctx context.Context, worker string, raw json.RawMessage, err error) {
	if worker == "" {
		worker = "unknown"
	}
	_ = os.MkdirAll(filepath.Join(p.Dir, "prompt"), 0o755)
	path := filepath.Join(p.Dir, "prompt", worker+".txt")

	var buf bytes.Buffer
	buf.WriteString("[RESPONSE]\n")
	if err != nil {
		buf.WriteString("ERROR: " + err.Error() + "\n\n")
	} else {
		buf.Write(raw)
		buf.WriteString("\n\n")
	}
	f, _ := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if f != nil {
		_, _ = f.Write(buf.Bytes())
		_ = f.Close()
	}

	_ = os.WriteFile(filepath.Join(p.Dir, worker+".raw.json"), raw, 0o644)
}
