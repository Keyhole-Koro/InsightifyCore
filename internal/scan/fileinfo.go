package scan

import (
    "errors"
    "io"
    "os"
    "path/filepath"
    "strings"
    "sync"

    t "insightify/internal/types"
)

// FileInfo returns a minimal index entry and (optionally) a preview of file content.
// - root: repository root (absolute or relative)
// - rel:  repo-relative path using '/' or filesystem separators
// - contentLimit: if > 0, up to N bytes of content are read; otherwise content is empty
func FileInfo(root, rel string, contentLimit int) (t.FileIndexEntry, string, error) {
    var out t.FileIndexEntry
    // Normalize inputs
    r := filepath.Clean(root)
    p := filepath.ToSlash(strings.TrimPrefix(filepath.Clean(rel), "./"))

    // Size (cached)
    sz, err := GetSize(r, p)
    if err != nil { return out, "", err }
    out = t.FileIndexEntry{
        Path:     p,
        Size:     sz,
        Language: "",
        Kind:     "",
        Ext:      strings.ToLower(filepath.Ext(p)),
    }
    if contentLimit <= 0 { return out, "", nil }
    prev, err := GetPreview(r, p, contentLimit)
    return out, prev, err
}

// -------- In-process file info cache (size + preview) --------

type fiEntry struct {
    size      int64
    hasSize   bool
    preview   string
    previewLen int
}

var (
    fiMu sync.RWMutex
    fiMap = map[string]*fiEntry{} // key: rootClean + "|" + repoRel
)

func cacheKey(rootClean, rel string) string {
    return filepath.ToSlash(filepath.Clean(rootClean)) + "|" + filepath.ToSlash(strings.TrimPrefix(filepath.Clean(rel), "./"))
}

// GetSize returns file size, caching the result.
func GetSize(root, rel string) (int64, error) {
    key := cacheKey(root, rel)
    fiMu.RLock()
    if e, ok := fiMap[key]; ok && e.hasSize {
        sz := e.size
        fiMu.RUnlock()
        return sz, nil
    }
    fiMu.RUnlock()

    abs := filepath.Join(filepath.Clean(root), filepath.FromSlash(filepath.ToSlash(rel)))
    st, err := os.Stat(abs)
    if err != nil { return 0, err }
    if st.IsDir() { return 0, errors.New("GetSize: path is a directory") }

    fiMu.Lock()
    e := fiMap[key]
    if e == nil { e = &fiEntry{}; fiMap[key] = e }
    e.size = st.Size()
    e.hasSize = true
    fiMu.Unlock()
    return e.size, nil
}

// GetPreview returns up to 'limit' bytes of file content, caching previews.
// If a shorter preview is cached and a larger limit is requested, it expands the cache.
func GetPreview(root, rel string, limit int) (string, error) {
    if limit <= 0 { return "", nil }
    key := cacheKey(root, rel)
    fiMu.RLock()
    if e, ok := fiMap[key]; ok && e.previewLen >= limit {
        s := e.preview[:limit]
        fiMu.RUnlock()
        return s, nil
    }
    fiMu.RUnlock()

    abs := filepath.Join(filepath.Clean(root), filepath.FromSlash(filepath.ToSlash(rel)))
    f, err := os.Open(abs)
    if err != nil { return "", err }
    defer f.Close()
    buf := make([]byte, limit)
    n, _ := io.ReadFull(f, buf)
    if n < 0 { n = 0 }
    s := string(buf[:n])

    fiMu.Lock()
    e := fiMap[key]
    if e == nil { e = &fiEntry{}; fiMap[key] = e }
    // Only expand preview if new limit is greater than what we have.
    if limit > e.previewLen {
        e.preview = s
        e.previewLen = limit
    }
    fiMu.Unlock()
    return s, nil
}

// ClearFileInfoCache clears the in-process file info cache.
func ClearFileInfoCache() {
    fiMu.Lock()
    fiMap = map[string]*fiEntry{}
    fiMu.Unlock()
}
