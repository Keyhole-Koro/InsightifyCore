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

// IndexRepo walks the repo and returns a file index and cleaned markdown docs.
func IndexRepo(root string) ([]t.FileIndexEntry, []t.MDDoc, error) {
	var index []t.FileIndexEntry
	var mdDocs []t.MDDoc

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Skip VCS & dependency dirs
			base := filepath.Base(path)
			switch base {
			case ".git", ".hg", ".svn", "node_modules", "vendor", "target", "build", ".next", ".cache":
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		ext := strings.ToLower(filepath.Ext(rel))
		size, _ := int64(0), 0
		if fi, e := os.Stat(path); e == nil {
			size = fi.Size()
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