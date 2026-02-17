package utils

import (
	"path"
	"path/filepath"
	"strings"
)

// BaseNames returns the last path element for each input path.
// It trims whitespace, normalizes separators, and skips empty results.
func BaseNames(paths ...string) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = filepath.ToSlash(p)
		p = strings.TrimRight(p, "/")
		if p == "" {
			continue
		}
		base := path.Base(p)
		if base == "." || base == "/" || base == "" {
			continue
		}
		out = append(out, base)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
