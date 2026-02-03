package wordidx

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"insightify/internal/safeio"
	"insightify/internal/scan"
)

/*
Package wordidx is a lightweight, word-only indexer.

Rules:
- Keep only ident-like words: start with Unicode letter or '_' and continue with letter/digit/'_'.
- Ignore numbers and symbols; quotes are delimiters.
- Positions are 1-based (line, column counted by runes).
- Aggregated index builds asynchronously via scan.ScanParallel; Find blocks until completion.
*/

// Word is a collected token and its position within a file.
type Word struct {
	Text string
	Line int
}

// Index holds words from a single file and a hash-based posting map.
type Index struct {
	Words []Word
	post  map[uint64][]int // hash -> indices into Words
}

// Build parses a single file content and collects words.
func Build(src []byte) *Index {
	idx := &Index{post: make(map[uint64][]int)}
	line, col := 1, 1

	isStart := func(r rune) bool { return r == '_' || unicode.IsLetter(r) }
	isCont := func(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

	i := 0
	for i < len(src) {
		r, w := utf8.DecodeRune(src[i:])
		if r == '\n' {
			line++
			col = 1
			i += w
			continue
		}
		if r == utf8.RuneError && w == 1 {
			// Treat invalid bytes as delimiters.
			i++
			col++
			continue
		}
		if isStart(r) {
			start := i
			i += w
			col++
			for i < len(src) {
				rc, wc := utf8.DecodeRune(src[i:])
				if rc == '\n' || !isCont(rc) {
					break
				}
				i += wc
				col++
			}
			word := string(src[start:i])
			idx.add(word, line)
			continue
		}
		// Delimiter: advance 1 rune.
		i += w
		col++
	}
	return idx
}

func (x *Index) add(word string, line int) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(word))
	key := h.Sum64()
	idx := len(x.Words)
	x.Words = append(x.Words, Word{Text: word, Line: line})
	if x.post == nil {
		x.post = make(map[uint64][]int)
	}
	x.post[key] = append(x.post[key], idx)
}

// Find returns positions of exact matches for the given word in this file index.
func (x *Index) Find(word string) []int {
	if x == nil || x.post == nil {
		return nil
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(word))
	key := h.Sum64()
	var out []int
	for _, i := range x.post[key] {
		if i >= 0 && i < len(x.Words) && x.Words[i].Text == word {
			out = append(out, x.Words[i].Line)
		}
	}
	return out
}

// FileIndex ties an Index with the file path it was built from.
type FileIndex struct {
	Path  string
	Index *Index
}

// PosRef ties a word occurrence to a file and its position.
type PosRef struct {
	FilePath string
	Line     int
}

// Builder allows fluent configuration of an AggIndex run.
type Builder struct {
	roots    []string
	workers  int
	allowExt []string
	opts     scan.Options
	err      error
	fs       *safeio.SafeFS
}

// New returns a Builder with sensible defaults (cache bypass and common ignores).
func New() *Builder {
	return &Builder{
		opts: scan.Options{
			IgnoreDirs:  []string{".git", "node_modules", "vendor"},
			BypassCache: true,
		},
	}
}

// Root sets one or more repository roots to index. Passing no arguments leaves
// the previously configured roots unchanged. A blank argument is ignored.
func (b *Builder) Root(paths ...string) *Builder {
	if b == nil {
		return b
	}
	if len(paths) == 0 {
		return b
	}
	var cleaned []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		resolved, err := scan.ResolveRoot(p)
		if err != nil {
			b.err = err
			return b
		}
		cleaned = append(cleaned, resolved)
	}
	if len(cleaned) == 0 {
		return b
	}
	b.roots = cleaned
	return b
}

// AddRoot appends another root to the existing list (duplicates are ignored).
func (b *Builder) AddRoot(path string) *Builder {
	if b == nil || path == "" {
		return b
	}
	p, err := scan.ResolveRoot(path)
	if err != nil {
		b.err = err
		return b
	}
	for _, existing := range b.roots {
		if existing == p {
			return b
		}
	}
	b.roots = append(b.roots, p)
	return b
}

// Repo resolves a repo name (folder inside scan.ReposDir) and adds it as a root.
func (b *Builder) Repo(name string) *Builder {
	if b == nil || name == "" {
		return b
	}
	return b.AddRoot(name)
}

// Allow restricts indexed files to the given extensions (e.g., "go", "ts").
func (b *Builder) Allow(exts ...string) *Builder {
	if b == nil {
		return b
	}
	if len(exts) == 0 {
		b.allowExt = nil
		return b
	}
	b.allowExt = append([]string(nil), exts...)
	return b
}

// Workers overrides the number of indexing workers. <=0 uses GOMAXPROCS.
func (b *Builder) Workers(n int) *Builder {
	if b == nil {
		return b
	}
	b.workers = n
	return b
}

// Options replaces the scan options used during traversal.
func (b *Builder) Options(opts scan.Options) *Builder {
	if b == nil {
		return b
	}
	b.opts = opts
	return b
}

// FS injects a SafeFS to use for file reads. Defaults to safeio.Default().
func (b *Builder) FS(fs *safeio.SafeFS) *Builder {
	if b == nil {
		return b
	}
	b.fs = fs
	return b
}

// Start kicks off indexing with the configured settings and returns the AggIndex.
func (b *Builder) Start(ctx context.Context) *AggIndex {
	if b == nil {
		return nil
	}
	if b.err != nil {
		agg := NewAgg()
		agg.setErr(b.err)
		agg.doneOnce.Do(func() { close(agg.doneCh) })
		return agg
	}
	if ctx == nil {
		ctx = context.Background()
	}
	roots := b.roots
	if len(roots) == 0 {
		b.err = errors.New("wordidx: no roots configured; call Root or Repo")
		agg := NewAgg()
		agg.setErr(b.err)
		agg.doneOnce.Do(func() { close(agg.doneCh) })
		return agg
	}
	roots = append([]string(nil), roots...) // avoid external mutation
	var filter func(scan.FileVisit) bool
	if len(b.allowExt) > 0 {
		filter = ExtAllow(b.allowExt...)
	}
	agg := NewAgg()
	if b.fs != nil {
		agg.fs = b.fs
	}
	agg.StartFromScans(ctx, roots, b.opts, b.workers, filter)
	return agg
}

// AggIndex aggregates indices across files asynchronously.
// StartFromScan begins indexing; Wait/Find synchronize on completion.
type AggIndex struct {
	mu       sync.RWMutex
	byHash   map[uint64][]PosRef // hash(word) -> postings across files
	files    []FileIndex
	doneOnce sync.Once
	doneCh   chan struct{}

	errMu    sync.Mutex
	firstErr error

	fs   *safeio.SafeFS
	fsMu sync.Mutex
}

// NewAgg creates an empty aggregator. Prefer Builder for fluent setup.
func NewAgg() *AggIndex {
	return &AggIndex{
		byHash: make(map[uint64][]PosRef),
		doneCh: make(chan struct{}),
	}
}

// StartFromScan indexes files discovered by a scan using a worker pool.
// fileFilter: optional predicate to include specific files (e.g., by extension).
// It returns immediately; Find/Wait can be used to await completion.
func (a *AggIndex) StartFromScan(ctx context.Context, root string, sopts scan.Options, workers int, fileFilter func(scan.FileVisit) bool) {
	a.StartFromScans(ctx, []string{root}, sopts, workers, fileFilter)
}

// StartFromScans indexes all provided roots sequentially using a shared worker pool.
// It returns immediately; Find/Wait can be used to await completion.
func (a *AggIndex) StartFromScans(ctx context.Context, roots []string, sopts scan.Options, workers int, fileFilter func(scan.FileVisit) bool) {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
		if workers <= 0 {
			workers = 1
		}
	}
	tasks := make(chan string, 256)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case p, ok := <-tasks:
					if !ok {
						return
					}
					a.indexOne(p)
				}
			}
		}()
	}
	go func() {
		defer func() {
			close(tasks)
			wg.Wait()
			a.doneOnce.Do(func() { close(a.doneCh) })
		}()
		for _, root := range roots {
			if ctx.Err() != nil {
				return
			}
			if root == "" {
				root = "."
			}
			root = filepath.Clean(root)
			err := scan.ScanWithOptions(root, sopts, func(fv scan.FileVisit) {
				if fv.IsDir {
					return
				}
				if fileFilter != nil && !fileFilter(fv) {
					return
				}
				select {
				case <-ctx.Done():
					return
				case tasks <- fv.AbsPath:
				}
			})
			if err != nil {
				a.setErr(err)
				return
			}
		}
	}()
}

// StartFromPaths indexes the provided absolute file paths. The call returns
// immediately; Wait/Find synchronize on completion.
func (a *AggIndex) StartFromPaths(ctx context.Context, paths []string, workers int) {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
		if workers <= 0 {
			workers = 1
		}
	}
	tasks := make(chan string, len(paths))
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case p, ok := <-tasks:
					if !ok {
						return
					}
					if p == "" {
						continue
					}
					a.indexOne(p)
				}
			}
		}()
	}
	go func() {
		defer func() {
			close(tasks)
			wg.Wait()
			a.doneOnce.Do(func() { close(a.doneCh) })
		}()
		for _, p := range paths {
			if p == "" {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case tasks <- p:
			}
		}
	}()
}

// Wait blocks until indexing is completed or ctx is canceled.
func (a *AggIndex) Wait(ctx context.Context) error {
	select {
	case <-a.doneCh:
		return a.getErr()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Find waits until indexing is completed, then returns postings for exact word matches.
// If the context is canceled before completion, it returns nil.
func (a *AggIndex) Find(ctx context.Context, word string) []PosRef {
	if err := a.Wait(ctx); err != nil {
		return nil
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(word))
	key := h.Sum64()

	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]PosRef, len(a.byHash[key]))
	copy(out, a.byHash[key])
	return out
}

// Files returns a snapshot of per-file indices (optional inspection).
func (a *AggIndex) Files(ctx context.Context) []FileIndex {
	_ = a.Wait(ctx)
	a.mu.RLock()
	defer a.mu.RUnlock()
	cp := make([]FileIndex, len(a.files))
	copy(cp, a.files)
	return cp
}

/* -------- internal helpers -------- */

func (a *AggIndex) indexOne(path string) {
	fs := a.safeFS()
	if fs == nil {
		return
	}
	data, err := fs.SafeReadFile(path)
	if err != nil {
		a.setErr(fmt.Errorf("wordidx: read %s: %w", path, err))
		return
	}
	idx := Build(data)

	a.mu.Lock()
	a.files = append(a.files, FileIndex{Path: path, Index: idx})
	for _, w := range idx.Words {
		h := fnv.New64a()
		_, _ = h.Write([]byte(w.Text))
		key := h.Sum64()
		a.byHash[key] = append(a.byHash[key], PosRef{FilePath: path, Line: w.Line})
	}
	a.mu.Unlock()
}

func (a *AggIndex) setErr(err error) {
	if err == nil {
		return
	}
	a.errMu.Lock()
	if a.firstErr == nil {
		a.firstErr = err
	}
	a.errMu.Unlock()
}
func (a *AggIndex) getErr() error {
	a.errMu.Lock()
	defer a.errMu.Unlock()
	return a.firstErr
}

func (a *AggIndex) safeFS() *safeio.SafeFS {
	if a == nil {
		if fs := scan.CurrentSafeFS(); fs != nil {
			return fs
		}
		return safeio.Default()
	}
	a.fsMu.Lock()
	defer a.fsMu.Unlock()
	if a.fs != nil {
		return a.fs
	}
	if dfs := scan.CurrentSafeFS(); dfs != nil {
		a.fs = dfs
		return dfs
	}
	if dfs := safeio.Default(); dfs != nil {
		a.fs = dfs
		return dfs
	}
	a.setErr(errors.New("wordidx: safe filesystem is not configured"))
	return nil
}

// ExtAllow returns a file filter that accepts only given extensions (e.g., "go","ts").
// Usage: wordidx.ExtAllow("go","ts","js")
func ExtAllow(exts ...string) func(scan.FileVisit) bool {
	allowed := map[string]struct{}{}
	for _, e := range exts {
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		allowed[strings.ToLower(e)] = struct{}{}
	}
	return func(fv scan.FileVisit) bool {
		if fv.IsDir {
			return false
		}
		ext := fv.Ext
		if ext == "" {
			ext = strings.ToLower(filepath.Ext(fv.AbsPath))
		}
		_, ok := allowed[ext]
		return ok
	}
}
