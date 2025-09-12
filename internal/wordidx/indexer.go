package wordidx

import (
	"context"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"insightify/internal/scan" // adjust to your module path
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
}

// NewAgg creates an empty aggregator.
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
		}
		close(tasks)
		wg.Wait()
		a.doneOnce.Do(func() { close(a.doneCh) })
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
	data, err := os.ReadFile(path)
	if err != nil {
		a.setErr(err)
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
