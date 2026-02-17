package mcp

import (
	"path/filepath"
	"strings"

	"insightify/internal/common/safeio"
)

// Host wires repo/artifact access for tools.
type Host struct {
	RepoRoot   string
	ReposRoot  string
	RepoFS     *safeio.SafeFS
	ArtifactFS *safeio.SafeFS
}

// RegisterDefaultTools installs the default tool set into a registry.
func RegisterDefaultTools(r *Registry, h Host) {
	if r == nil {
		return
	}
	r.Register(newScanListTool(h))
	r.Register(newFSReadTool(h))
	r.Register(newWordIdxSearchTool(h))
	r.Register(newSnippetCollectTool(h))
	r.Register(newDeltaDiffTool())
	r.Register(newGitHubCloneTool(h))
}

func resolveRepoPath(repoRoot, rel string) string {
	if strings.TrimSpace(rel) == "" {
		return repoRoot
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	if repoRoot == "" {
		return rel
	}
	return filepath.Join(repoRoot, rel)
}
