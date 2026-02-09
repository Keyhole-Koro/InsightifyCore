package codebase

import (
	"path/filepath"
	"strings"
)

// FileRef captures common metadata about a repository file and avoids repeatedly
// slicing path strings in downstream stages.
type FileRef struct {
	Path string `json:"path"`          // repo-relative path using forward slashes
	Dir  string `json:"dir,omitempty"` // directory portion ("" for repo root)
	Base string `json:"base"`          // filename with extension
	Name string `json:"name"`          // filename without extension
	Ext  string `json:"ext,omitempty"` // extension without dot (lowercase)
}

// NewFileRef normalizes a repository-relative path and precomputes useful fields.
func NewFileRef(path string) FileRef {
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." {
		clean = ""
	}
	base := filepath.Base(clean)
	if base == "." && clean == "" {
		base = ""
	}
	dir := filepath.Dir(clean)
	if dir == "." {
		dir = ""
	} else {
		dir = filepath.ToSlash(dir)
	}
	extWithDot := filepath.Ext(base)
	ext := strings.TrimPrefix(extWithDot, ".")
	name := base
	if extWithDot != "" && len(base) > len(extWithDot) {
		name = base[:len(base)-len(extWithDot)]
	}
	return FileRef{
		Path: clean,
		Dir:  dir,
		Base: base,
		Name: name,
		Ext:  strings.ToLower(ext),
	}
}
