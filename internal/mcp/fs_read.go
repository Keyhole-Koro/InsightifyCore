package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"insightify/internal/safeio"
)

// --------------------- fs.read ---------------------

type fsReadTool struct{ host Host }

func newFSReadTool(h Host) *fsReadTool { return &fsReadTool{host: h} }

func (t *fsReadTool) Spec() ToolSpec {
	return ToolSpec{
		Name:        "fs.read",
		Description: "Read a file (or a slice) from the repo, with size limits.",
	}
}

type fsReadInput struct {
	Path   string `json:"path"`
	Start  int64  `json:"start"`
	Length int64  `json:"length"`
}

type fsReadOutput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (t *fsReadTool) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in fsReadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Path) == "" {
		return nil, fmt.Errorf("fs.read: path required")
	}
	if in.Length <= 0 {
		in.Length = 65536
	}
	fs := t.host.RepoFS
	if fs == nil {
		fs = safeio.Default()
	}
	if fs == nil {
		return nil, fmt.Errorf("fs.read: repo fs not configured")
	}
	f, err := fs.SafeOpen(in.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if in.Start > 0 {
		if _, err := f.Seek(in.Start, io.SeekStart); err != nil {
			return nil, err
		}
	}
	buf, err := io.ReadAll(io.LimitReader(f, in.Length))
	if err != nil {
		return nil, err
	}
	out := fsReadOutput{Path: in.Path, Content: string(buf)}
	return json.Marshal(out)
}
