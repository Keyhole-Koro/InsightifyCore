package scan

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// FilesWithExtensions walks root and returns repo-relative paths of files whose
// extensions match any entry in exts. Extensions are case-insensitive and may
// be provided with or without a leading dot. The supplied options are copied;
// the scan always bypasses caches to ensure fresh results.
func FilesWithExtensions(root string, exts []string, opts Options) ([]string, error) {
	if len(exts) == 0 {
		return nil, nil
	}

	allowed := make(map[string]struct{}, len(exts))
	for _, ext := range exts {
		ext = strings.TrimSpace(ext)
		if ext == "" {
			continue
		}
		ext = strings.ToLower(ext)
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		allowed[ext] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, nil
	}

	// Copy options to avoid mutating caller state.
	scanOpts := opts
	scanOpts.BypassCache = true

	var (
		mu    sync.Mutex
		files []string
	)
	err := ScanWithOptions(root, scanOpts, func(fv FileVisit) {
		if fv.IsDir {
			return
		}
		ext := fv.Ext
		if ext == "" {
			ext = filepath.Ext(fv.Path)
		}
		if ext == "" {
			return
		}
		ext = strings.ToLower(ext)
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		if _, ok := allowed[ext]; !ok {
			return
		}
		mu.Lock()
		files = append(files, filepath.ToSlash(fv.Path))
		mu.Unlock()
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}
