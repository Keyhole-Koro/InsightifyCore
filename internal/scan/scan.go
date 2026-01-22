package scan

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
)

/*
Package scan walks a repository tree and emits lightweight metadata for each entry.
It supports:
  - MaxDepth (limit descent),
  - IgnoreDirs (skip by basename),
  - In-process caching.

	Optional subtree caching enables partial re-scan for specific folders.
*/

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
type VisitFunc func(f FileVisit)

// Options customizes the scanning behavior.
type Options struct {
	// MaxDepth limits how deep under root we descend. 0 or negative means unlimited.
	// Depth is counted by slashes under root: root=0, root/a=1, root/a/b=2, ...
	MaxDepth int
	// IgnoreDirs is a list of directory basenames to skip entirely (e.g., "node_modules").
	IgnoreDirs []string

	// BypassCache forces a full scan, ignoring caches.
	BypassCache bool
	// CacheSubtrees enables directory-subtree caching, allowing partial re-scans.
	CacheSubtrees bool
	// ChangedPrefixes invalidates cached subtrees for the given repo-relative prefixes
	// (e.g., "src/featureA"). Only useful when CacheSubtrees is true.
	ChangedPrefixes []string
}

// Scan walks the repo and invokes cb for each visited entry (dirs and files).
// Use ScanWithOptions for depth limiting, directory ignores, or subtree caching.
func Scan(root string, cb VisitFunc) error {
	return ScanWithOptions(root, Options{}, cb)
}

// ScanWithOptions walks the repo with options controlling depth/ignores/caching.
// If CacheSubtrees is false, it uses whole-tree caching (compatible with previous behavior).
// If CacheSubtrees is true, it uses subtree caching and can re-scan only changed prefixes.
func ScanWithOptions(root string, opts Options, cb VisitFunc) error {
	resolved, err := ResolveRoot(root)
	if err != nil {
		return err
	}
	rClean := filepath.Clean(resolved)

	fsys := safeFS()
	if fi, err := fsys.SafeStat(rClean); err != nil {
		abs, _ := filepath.Abs(rClean)
		return fmt.Errorf("scan: root not found or not readable: %s (abs=%s): %w", rClean, abs, err)
	} else if !fi.IsDir() {
		abs, _ := filepath.Abs(rClean)
		return fmt.Errorf("scan: root is not a directory: %s (abs=%s)", rClean, abs)
	}
	// If subtree caching is disabled and not bypassed, fall back to the original whole-cache path.
	if !opts.CacheSubtrees && !opts.BypassCache {
		key := wholeCacheKey(rClean, opts)
		if items, ok := getWholeCache(key); ok {
			for _, it := range items {
				if cb != nil {
					cb(it)
				}
			}
			return nil
		}
		// Miss: walk with WalkDir and populate.
		var items []FileVisit
		err := filepath.WalkDir(rClean, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // swallow and continue
			}
			rel, _ := filepath.Rel(rClean, path)
			rel = filepath.ToSlash(rel)
			depth := slashCount(rel)

			if d.IsDir() {
				base := filepath.Base(path)
				for _, ig := range opts.IgnoreDirs {
					if ig != "" && base == ig {
						return filepath.SkipDir
					}
				}
				if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
					return filepath.SkipDir
				}
			}

			ext := strings.ToLower(filepath.Ext(rel))
			size := int64(0)
			if !d.IsDir() {
				if fi, e := d.Info(); e == nil {
					size = fi.Size()
				} else if fi, e2 := fsys.SafeStat(path); e2 == nil {
					size = fi.Size()
				}
			}
			fv := FileVisit{Path: rel, AbsPath: path, IsDir: d.IsDir(), Ext: ext, Size: size}
			items = append(items, fv)
			if cb != nil {
				cb(fv)
			}
			return nil
		})
		if err == nil {
			putWholeCache(key, items)
		}
		return err
	}

	// Subtree caching mode
	if opts.CacheSubtrees {
		// Normalize ignore list and changed prefixes; then re-scan only what is necessary.
		ig := normalizeIgnores(opts.IgnoreDirs)
		igKey := strings.Join(ig, ",")
		// Invalidate requested prefixes (if any)
		for _, p := range opts.ChangedPrefixes {
			p = cleanRel(p)
			if p == "." || p == "" {
				continue
			}
			invalidatePrefix(rClean, p, opts.MaxDepth, igKey)
		}
		// BypassCache means: don't use subtree cache at all; do full recursive traversal and overwrite caches.
		// Run traversal in parallel.
		pc := newParallelCtx()
		err = walkSubtreeCached(rClean, ".", 0, opts.MaxDepth, ig, igKey, cb, opts.BypassCache, pc)
		pc.wg.Wait()
		if err == nil {
			err = pc.getErr()
		}
		return err
	}

	// Fallback: full re-scan without caching, in parallel
	pc := newParallelCtx()
	err = walkSubtreeCached(rClean, ".", 0, opts.MaxDepth, normalizeIgnores(opts.IgnoreDirs), "", cb, true, pc)
	pc.wg.Wait()
	if err == nil {
		err = pc.getErr()
	}
	return err
}

/* ---------------- In-process caches ---------------- */

var (
	cacheMu sync.RWMutex

	// Whole-tree cache: key = root|MaxDepth|sorted(ignore)
	wholeCache = map[string][]FileVisit{}

	// Subtree cache: key = root|prefix|remainDepth|sorted(ignore)
	// Values store paths relative to the prefix (with "." for the directory itself).
	subtreeCache = map[string][]FileVisit{}
)

func wholeCacheKey(root string, opts Options) string {
	ig := normalizeIgnores(opts.IgnoreDirs)
	return strings.Join([]string{
		filepath.ToSlash(root),
		strconv.Itoa(opts.MaxDepth),
		strings.Join(ig, ","),
	}, "|")
}

func subtreeKey(root, prefix string, remainDepth int, ignoreKey string) string {
	return strings.Join([]string{
		filepath.ToSlash(root),
		filepath.ToSlash(prefix),
		strconv.Itoa(remainDepth),
		ignoreKey,
	}, "|")
}

func getWholeCache(key string) ([]FileVisit, bool) {
	cacheMu.RLock()
	v, ok := wholeCache[key]
	cacheMu.RUnlock()
	return v, ok
}
func putWholeCache(key string, v []FileVisit) {
	cacheMu.Lock()
	wholeCache[key] = v
	cacheMu.Unlock()
}

func getSubtreeCache(key string) ([]FileVisit, bool) {
	cacheMu.RLock()
	v, ok := subtreeCache[key]
	cacheMu.RUnlock()
	return v, ok
}
func putSubtreeCache(key string, v []FileVisit) {
	cacheMu.Lock()
	subtreeCache[key] = v
	cacheMu.Unlock()
}

// ClearCache removes all cached data (both whole-tree and subtree caches).
func ClearCache() {
	cacheMu.Lock()
	wholeCache = map[string][]FileVisit{}
	subtreeCache = map[string][]FileVisit{}
	cacheMu.Unlock()
}

// InvalidatePrefix removes cached subtrees matching (root, prefix) for any remaining depth.
func InvalidatePrefix(root, prefix string, opts Options) {
	ig := normalizeIgnores(opts.IgnoreDirs)
	igKey := strings.Join(ig, ",")
	invalidatePrefix(filepath.Clean(root), cleanRel(prefix), opts.MaxDepth, igKey)
}

func invalidatePrefix(root, prefix string, maxDepth int, ignoreKey string) {
	root = filepath.Clean(root)
	prefix = cleanRel(prefix)

	cacheMu.Lock()
	for k := range subtreeCache {
		// key layout: root|prefix|remainDepth|ignoreKey
		if strings.HasPrefix(k, filepath.ToSlash(root)+"|"+filepath.ToSlash(prefix)+"|") &&
			(strings.HasSuffix(k, "|"+ignoreKey) || ignoreKey == "") {
			delete(subtreeCache, k)
		}
	}
	cacheMu.Unlock()
}

/* ---------------- Subtree-walking with partial caching ---------------- */

// walkSubtreeCached recursively traverses from relPrefix, using subtree cache per-directory.
// If bypass is true, the cache is ignored and overwritten.
type parallelCtx struct {
	wg  *sync.WaitGroup
	sem chan struct{}
	mu  sync.Mutex
	err error
}

func newParallelCtx() *parallelCtx {
	// Limit parallelism to a reasonable number relative to CPUs.
	n := runtime.GOMAXPROCS(0)
	if n < 2 {
		n = 2
	}
	// Allow a bit more concurrency for IO-bound directory reads.
	limit := n * 4
	return &parallelCtx{wg: &sync.WaitGroup{}, sem: make(chan struct{}, limit)}
}

func (p *parallelCtx) setErr(e error) {
	if e == nil {
		return
	}
	p.mu.Lock()
	if p.err == nil {
		p.err = e
	}
	p.mu.Unlock()
}
func (p *parallelCtx) getErr() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

func walkSubtreeCached(root string, relPrefix string, depth int, maxDepth int, ignores []string, ignoreKey string, cb VisitFunc, bypass bool, pc *parallelCtx) error {
	fs := safeFS()
	abs := joinAbs(root, relPrefix)
	isRoot := relPrefix == "." || relPrefix == ""

	// Decide if this directory should be skipped due to ignore or depth.
	if !isRoot {
		base := filepath.Base(abs)
		for _, ig := range ignores {
			if ig != "" && base == ig {
				return nil // skip entire subtree; do not emit the directory itself
			}
		}
		if maxDepth > 0 && depth > maxDepth { // depth check: depth==1 means "root/a"
			return nil
		}
	}

	// Compute remaining depth allowance from this point
	remain := -1
	if maxDepth > 0 {
		remain = maxDepth - depth
		if remain < 0 {
			remain = 0
		}
	}

	// Serve from subtree cache if available and not bypassed
	if !bypass && !isRoot {
		key := subtreeKey(root, relPrefix, remain, ignoreKey)
		if items, ok := getSubtreeCache(key); ok {
			for _, it := range items {
				emitWithPrefix(root, relPrefix, it, cb)
			}
			return nil
		}
	}

	// Not cached: list this directory and recursively visit children
	entries, err := fs.SafeReadDir(abs)
	if err != nil {
		return nil // swallow and continue
	}

	var collected []FileVisit

	// Emit/collect the directory itself (to match previous behavior which included dirs)
	dirVisit := FileVisit{
		Path:    relPrefixIf(isRoot, "."),
		AbsPath: abs,
		IsDir:   true,
		Ext:     "",
		Size:    0,
	}
	if !isRoot { // for root ".", we do not emit "." as a separate entry
		collected = append(collected, dirVisit)
		emitWithPrefix(root, relPrefix, dirVisit, cb)
	}

	for _, e := range entries {
		childRel := joinRel(relPrefix, e.Name())
		childAbs := filepath.Join(abs, e.Name())
		relForExt := childRel
		ext := strings.ToLower(filepath.Ext(relForExt))

		if e.IsDir() {
			// Depth check for child
			childDepth := depth + 1
			if maxDepth > 0 && childDepth >= maxDepth {
				// Do not descend further, but still emit the directory node (already handled above only for relPrefix)
				// Here we still want to emit this child directory itself as a node.
				dirNode := FileVisit{Path: ".", AbsPath: childAbs, IsDir: true, Ext: "", Size: 0}
				emitWithPrefix(root, childRel, dirNode, cb)
				collected = append(collected, dirNode)
				continue
			}
			// Recurse into child (this will handle ignore checks and caching for that child)
			if pc != nil {
				pc.wg.Add(1)
				pc.sem <- struct{}{}
				go func(cr string) {
					defer pc.wg.Done()
					defer func() { <-pc.sem }()
					if e := walkSubtreeCached(root, cr, childDepth, maxDepth, ignores, ignoreKey, cb, bypass, pc); e != nil {
						pc.setErr(e)
					}
				}(childRel)
			} else {
				if err := walkSubtreeCached(root, childRel, childDepth, maxDepth, ignores, ignoreKey, cb, bypass, nil); err != nil {
					return err
				}
			}
		} else {
			var size int64
			if fi, e2 := e.Info(); e2 == nil {
				size = fi.Size()
			} else if fi, e3 := fs.SafeStat(childAbs); e3 == nil {
				size = fi.Size()
			}

			fileNode := FileVisit{
				Path:    filepath.Base(childRel), // relative to prefix; fixed by emitWithPrefix
				AbsPath: childAbs,
				IsDir:   false,
				Ext:     ext,
				Size:    size,
			}
			emitWithPrefix(root, relPrefix, fileNode, cb)
			collected = append(collected, fileNode)
		}
	}

	// Store subtree cache for this directory (except root ".")
	if !isRoot && !bypass {
		key := subtreeKey(root, relPrefix, remain, ignoreKey)
		putSubtreeCache(key, collected)
	}
	return nil
}

/* ---------------- Small helpers ---------------- */

func slashCount(rel string) int {
	if rel == "." || rel == "" {
		return 0
	}
	n := 0
	for i := 0; i < len(rel); i++ {
		if rel[i] == '/' {
			n++
		}
	}
	return n + 1
}

// normalizeIgnores trims and sorts ignore basenames.
func normalizeIgnores(in []string) []string {
	ig := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		ig = append(ig, s)
	}
	sort.Strings(ig)
	return ig
}

func cleanRel(p string) string {
	p = filepath.ToSlash(filepath.Clean(p))
	if p == "." {
		return "."
	}
	return strings.TrimPrefix(p, "./")
}

func joinAbs(root, rel string) string {
	if rel == "." || rel == "" {
		return root
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}

func joinRel(prefix, name string) string {
	if prefix == "." || prefix == "" {
		return name
	}
	return filepath.ToSlash(filepath.Join(prefix, name))
}

func relPrefixIf(isRoot bool, s string) string {
	if isRoot {
		return "."
	}
	return s
}

// emitWithPrefix converts a subtree-relative FileVisit into repo-relative, then invokes cb.
func emitWithPrefix(root, prefix string, it FileVisit, cb VisitFunc) {
	if cb == nil {
		return
	}
	// Rebuild repo-relative path: prefix + it.Path (handling "." specially)
	var rel string
	if it.Path == "." || it.Path == "" {
		rel = prefix
	} else if prefix == "." || prefix == "" {
		rel = it.Path
	} else {
		rel = filepath.ToSlash(filepath.Join(prefix, it.Path))
	}
	abs := joinAbs(root, rel)
	out := FileVisit{
		Path:    rel,
		AbsPath: abs,
		IsDir:   it.IsDir,
		Ext:     it.Ext,
		Size:    it.Size,
	}
	cb(out)
}
