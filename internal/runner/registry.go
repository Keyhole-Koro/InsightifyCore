package runner

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"insightify/internal/llm"
	t "insightify/internal/types"
	"insightify/internal/wordidx"
)

// Env is the shared environment passed to builders/runners.
type Env struct {
	Repo      string
	OutDir    string
	MaxNext   int
	ModelSalt string
	ForceFrom string

	LLM llm.LLMClient

	WordIndexer wordidx.AggIndex

	Index  []t.FileIndexEntry
	MDDocs []t.MDDoc

	StripImgMD   *regexp.Regexp
	StripImgHTML *regexp.Regexp
}

// PhaseSpec declares "what" a phase needs, not "how" the app calls it.
type PhaseSpec struct {
	Key         string                                           // e.g. "m0"
	File        string                                           // e.g. "m0.json"
	BuildInput  func(ctx context.Context, env *Env) (any, error) // produce logical input
	Run         func(ctx context.Context, in any, env *Env) (any, error)
	Fingerprint func(in any, env *Env) string // stable hash for caching
	Downstream  []string                      // phases to invalidate when forced
	Strategy    CacheStrategy                 // how to cache (json, versioned, none)
}

// CacheStrategy abstracts artifact persistence policies (json, versioned, …).
type CacheStrategy interface {
	// TryLoad returns (out, true) if cache hit and not forced.
	TryLoad(ctx context.Context, spec PhaseSpec, env *Env, inputFP string) (any, bool)
	// Save persists result and metadata.
	Save(ctx context.Context, spec PhaseSpec, env *Env, out any, inputFP string) error
	// Invalidate removes outputs/meta for this phase (used for downstream invalidation).
	Invalidate(ctx context.Context, spec PhaseSpec, env *Env) error
}

// --------------------- JSON file strategy (m0/m1/m2/x1) ---------------------

type jsonStrategy struct{}

type cacheMeta struct {
	Inputs    string    `json:"inputs"`
	Salt      string    `json:"salt,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (jsonStrategy) metaPath(spec PhaseSpec, env *Env) string {
	return filepath.Join(env.OutDir, spec.Key+".meta.json")
}
func (jsonStrategy) outPath(spec PhaseSpec, env *Env) string {
	return filepath.Join(env.OutDir, spec.File)
}

func (s jsonStrategy) TryLoad(ctx context.Context, spec PhaseSpec, env *Env, inputFP string) (any, bool) {
	if env.ForceFrom != "" && env.ForceFrom == strings.ToLower(spec.Key) {
		return nil, false
	}
	mp, op := s.metaPath(spec, env), s.outPath(spec, env)
	if !FileExists(mp) || !FileExists(op) {
		return nil, false
	}
	var m cacheMeta
	if b, err := os.ReadFile(mp); err == nil && json.Unmarshal(b, &m) == nil {
		if m.Inputs == inputFP && m.Salt == env.ModelSalt {
			var out any
			if b, err := os.ReadFile(op); err == nil && json.Unmarshal(b, &out) == nil {
				log.Printf("%s: using cache → %s", strings.ToUpper(spec.Key), op)
				return out, true
			}
		}
	}
	return nil, false
}

func (s jsonStrategy) Save(ctx context.Context, spec PhaseSpec, env *Env, out any, inputFP string) error {
	mp, op := s.metaPath(spec, env), s.outPath(spec, env)
	if b, e := json.MarshalIndent(out, "", "  "); e == nil {
		_ = os.WriteFile(op, b, 0o644)
	}
	mb, _ := json.MarshalIndent(cacheMeta{Inputs: inputFP, Salt: env.ModelSalt, CreatedAt: time.Now()}, "", "  ")
	_ = os.WriteFile(mp, mb, 0o644)
	log.Printf("%s → %s", strings.ToUpper(spec.Key), op)
	return nil
}

func (s jsonStrategy) Invalidate(ctx context.Context, spec PhaseSpec, env *Env) error {
	_ = os.Remove(s.outPath(spec, env))
	_ = os.Remove(s.metaPath(spec, env))
	return nil
}

// --------------------- Versioned JSON strategy (x0) -------------------------

// versionedStrategy always writes a new versioned file (x0_vN.json) and updates x0.json.
// Cache read is intentionally disabled (x0 is exploratory / append-only).
type versionedStrategy struct{}

func (versionedStrategy) TryLoad(ctx context.Context, spec PhaseSpec, env *Env, inputFP string) (any, bool) {
	// Never reuse cache for versioned phases (consistent with previous x0 behavior).
	return nil, false
}

func (versionedStrategy) Save(ctx context.Context, spec PhaseSpec, env *Env, out any, inputFP string) error {
	// Always start at v1 for each run; overwrite v1 and latest, and optionally prune older versions.
	versioned := fmt.Sprintf("%s_v1.json", spec.Key)
	versionedPath := filepath.Join(env.OutDir, versioned)
	latestPath := filepath.Join(env.OutDir, spec.File)

	if b, e := json.MarshalIndent(out, "", "  "); e == nil {
		_ = os.WriteFile(versionedPath, b, 0o644)
		_ = os.WriteFile(latestPath, b, 0o644)
	}
	// meta is optional for versioned write; record last inputs for debugging
	mp := filepath.Join(env.OutDir, spec.Key+".meta.json")
	mb, _ := json.MarshalIndent(cacheMeta{Inputs: inputFP, Salt: env.ModelSalt, CreatedAt: time.Now()}, "", "  ")
	_ = os.WriteFile(mp, mb, 0o644)

	// Best-effort pruning of other versions (x0_vN.json where N != 1)
	entries, _ := os.ReadDir(env.OutDir)
	re := regexp.MustCompile(fmt.Sprintf(`^%s_v(\d+)\.json$`, regexp.QuoteMeta(spec.Key)))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if m := re.FindStringSubmatch(name); len(m) == 2 && name != versioned {
			_ = os.Remove(filepath.Join(env.OutDir, name))
		}
	}
	log.Printf("%s → %s (reset to v1; updated %s)", strings.ToUpper(spec.Key), versionedPath, latestPath)
	return nil
}

func (versionedStrategy) Invalidate(ctx context.Context, spec PhaseSpec, env *Env) error {
	// No-op: x0 keeps versions; do not delete history. Keep spec.File (latest) as well.
	return nil
}

// --------------------- Execution with force+cache middlewares ----------------

// ExecutePhase builds input, applies force-from + strategy caching, runs, then invalidates downstream.
func ExecutePhase(ctx context.Context, spec PhaseSpec, env *Env, registry map[string]PhaseSpec) error {
	// Build logical input
	in, err := spec.BuildInput(ctx, env)
	if err != nil {
		return err
	}

	// Compute fingerprint
	fp := spec.Fingerprint(in, env)

	// Try cache (if strategy supports it)
	if out, ok := spec.Strategy.TryLoad(ctx, spec, env, fp); ok {
		_ = out // nothing else to do
		return nil
	}

	// Run phase
	out, err := spec.Run(ctx, in, env)
	if err != nil {
		return err
	}

	// Persist artifact via strategy
	if err := spec.Strategy.Save(ctx, spec, env, out, fp); err != nil {
		return err
	}

	// If forced, invalidate downstream caches (json-strategy only).
	if env.ForceFrom != "" && env.ForceFrom == strings.ToLower(spec.Key) {
		for _, d := range spec.Downstream {
			if ds, ok := registry[d]; ok {
				_ = ds.Strategy.Invalidate(ctx, ds, env)
			}
		}
	}
	return nil
}

// --------------------- Common helpers (hash, json, files) -------------------

func JSONFingerprint(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:])[:16]
}

func FileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func ReadJSON(dir, name string, v any) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		log.Fatalf("failed to read %s: %v", name, err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		log.Fatalf("failed to unmarshal %s: %v\nraw: %s", name, err, string(b))
	}
}

func WriteJSON(dir, name string, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, name), b, 0o644)
}

func NextVersion(outDir, key string) int {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return 1
	}
	re := regexp.MustCompile(fmt.Sprintf(`^%s_v(\d+)\.json$`, regexp.QuoteMeta(key)))
	max := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := re.FindStringSubmatch(e.Name())
		if len(m) == 2 {
			var n int
			_, _ = fmt.Sscanf(m[1], "%d", &n)
			if n > max {
				max = n
			}
		}
	}
	return max + 1
}

// Utility transforms used in several phases

func UniqueStrings(in ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func FilterIndexByRoots(index []t.FileIndexEntry, roots []string) []t.FileIndexEntry {
	if len(roots) == 0 {
		return index
	}
	var out []t.FileIndexEntry
	for _, it := range index {
		for _, r := range roots {
			r = strings.TrimSuffix(strings.TrimPrefix(r, "/"), "/")
			if r == "" {
				continue
			}
			if strings.HasPrefix(strings.ToLower(it.Path), strings.ToLower(r+"/")) || strings.EqualFold(it.Path, r) {
				out = append(out, it)
				break
			}
		}
	}
	return out
}

func FilterMDDocsByRoots(docs []t.MDDoc, roots []string) []t.MDDoc {
	if len(roots) == 0 {
		return docs
	}
	var out []t.MDDoc
	for _, d := range docs {
		for _, r := range roots {
			r = strings.TrimSuffix(strings.TrimPrefix(r, "/"), "/")
			if r == "" {
				continue
			}
			if strings.HasPrefix(strings.ToLower(d.Path), strings.ToLower(r+"/")) || strings.EqualFold(d.Path, r) {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// baseNames returns the final path segment for each provided path.
// Inputs may be repo-relative or absolute; empty segments are ignored.
func baseNames(paths ...string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		b := filepath.Base(filepath.ToSlash(p))
		if b != "" {
			out = append(out, b)
		}
	}
	return out
}
