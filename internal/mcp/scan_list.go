package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"insightify/internal/artifact"
	"insightify/internal/common/scan"
)

// --------------------- scan.list ---------------------

type scanListTool struct{ host Host }

func newScanListTool(h Host) *scanListTool { return &scanListTool{host: h} }

func (t *scanListTool) Spec() artifact.ToolSpec {
	return artifact.ToolSpec{
		Name:        "scan.list",
		Description: "List files under repo roots with optional depth and extension filters.",
	}
}

type scanListInput struct {
	Roots    []string `json:"roots"`
	MaxDepth int      `json:"max_depth"`
	AllowExt []string `json:"allow_ext"`
	MaxFiles int      `json:"max_files"`
}

type scanListOutput struct {
	Files []scanFile `json:"files"`
}

type scanFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Ext       string `json:"ext"`
}

func (t *scanListTool) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in scanListInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}
	if len(in.Roots) == 0 {
		return nil, fmt.Errorf("scan.list: roots required")
	}
	if in.MaxFiles <= 0 {
		in.MaxFiles = 2000
	}
	allow := make(map[string]struct{}, len(in.AllowExt))
	for _, e := range in.AllowExt {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" && !strings.HasPrefix(e, ".") {
			if ext := filepath.Ext(e); ext != "" {
				e = ext
			} else {
				e = "." + e
			}
		}
		if e != "" {
			allow[e] = struct{}{}
		}
	}
	var out scanListOutput
	appendFile := func(f scan.FileVisit) {
		if f.IsDir {
			return
		}
		if len(allow) > 0 {
			if _, ok := allow[f.Ext]; !ok {
				return
			}
		}
		out.Files = append(out.Files, scanFile{Path: f.Path, SizeBytes: f.Size, Ext: f.Ext})
	}
	for _, root := range in.Roots {
		if len(out.Files) >= in.MaxFiles {
			break
		}
		rootPath := resolveRepoPath(t.host.RepoRoot, root)
		_ = scan.ScanWithOptions(rootPath, scan.Options{MaxDepth: in.MaxDepth}, func(f scan.FileVisit) {
			if len(out.Files) >= in.MaxFiles {
				return
			}
			appendFile(f)
		})
	}
	return json.Marshal(out)
}
