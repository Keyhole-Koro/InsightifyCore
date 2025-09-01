package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	t "insightify/internal/types"
)

var (
	reImgMD   = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	reImgHTML = regexp.MustCompile(`(?is)<img[^>]*>`)
)

// FileVisit carries per-entry metadata to user callbacks.
type FileVisit struct {
	// Repo-relative path using forward slashes (e.g., "src/app.go").
	Path string
	// Absolute filesystem path.
	AbsPath string
	// True when the entry is a directory.
	IsDir bool
	// Lowercased extension (e.g., ".go", ".md"); empty for dirs or no-ext files.
	Ext string
	// File size in bytes; 0 for dirs or when stat fails.
	Size int64
}

// VisitFunc is an optional callback invoked for every visited entry.
// Use a closure to accumulate custom stats (e.g., extension counts).
type VisitFunc func(f FileVisit)

// IndexRepo walks the repo and returns a file index and cleaned markdown docs.
// It is equivalent to IndexRepoWithCallback(root, nil).
func IndexRepo(root string) ([]t.FileIndexEntry, []t.MDDoc, error) {
	return IndexRepoWithCallback(root, nil)
}

// IndexRepoWithCallback walks the repo and also invokes cb for each visited
// entry (dirs and files), allowing callers to compute custom analytics.
func IndexRepoWithCallback(root string, cb VisitFunc) ([]t.FileIndexEntry, []t.MDDoc, error) {
	var index []t.FileIndexEntry
	var mdDocs []t.MDDoc

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Skip VCS & dependency dirs
		if d.IsDir() {
			base := filepath.Base(path)
			switch base {
			case ".git", ".hg", ".svn", "node_modules", "vendor", "target", "build", ".next", ".cache":
				return filepath.SkipDir
			}
		}

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		ext := strings.ToLower(filepath.Ext(rel))
		size := int64(0)
		if !d.IsDir() {
			if fi, e := os.Stat(path); e == nil {
				size = fi.Size()
			}
		}

		if cb != nil {
			cb(FileVisit{Path: rel, AbsPath: path, IsDir: d.IsDir(), Ext: ext, Size: size})
		}

		// Build index and docs for files only
		if d.IsDir() {
			return nil
		}

		index = append(index, t.FileIndexEntry{
			Path: rel,
			Size: size,
			// Language and Kind left empty; can be inferred later if needed.
		})

		if ext == ".md" {
			if b, e := os.ReadFile(path); e == nil {
				txt := string(b)
				txt = reImgMD.ReplaceAllString(txt, "")
				txt = reImgHTML.ReplaceAllString(txt, "")
				mdDocs = append(mdDocs, t.MDDoc{Path: rel, Text: txt})
			}
		}
		return nil
	})
	return index, mdDocs, err
}
