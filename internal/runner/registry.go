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

	llmclient "insightify/internal/llmClient"
	"insightify/internal/pipeline/plan"
	"insightify/internal/safeio"
	t "insightify/internal/types"
	"insightify/internal/wordidx"
)

// SpecResolver resolves phase keys to specs, enabling cross-registry lookup.
type SpecResolver interface {
	Get(key string) (PhaseSpec, bool)
}

// MapResolver is a simple SpecResolver backed by a map keyed by normalized phase keys.
type MapResolver struct {
	specs map[string]PhaseSpec
}

// Get returns the PhaseSpec for the provided key, if present.
func (r MapResolver) Get(key string) (PhaseSpec, bool) {
	if len(r.specs) == 0 {
		return PhaseSpec{}, false
	}
	spec, ok := r.specs[normalizeKey(key)]
	return spec, ok
}

// MergeRegistries flattens multiple phase registries into a single resolver.
func MergeRegistries(regs ...map[string]PhaseSpec) SpecResolver {
	merged := make(map[string]PhaseSpec, 16)
	for _, reg := range regs {
		for k, v := range reg {
			merged[normalizeKey(k)] = v
		}
	}
	return MapResolver{specs: merged}
}

// Env is the shared environment passed to builders/runners.
type Env struct {
	Repo       string
	RepoRoot   string
	OutDir     string
	MaxNext    int
	RepoFS     *safeio.SafeFS
	ArtifactFS *safeio.SafeFS
	Resolver   SpecResolver

	ModelSalt string
	ForceFrom string

	LLM llmclient.LLMClient

	WordIndexer wordidx.AggIndex

	Index  []t.FileIndexEntry
	MDDocs []t.MDDoc

	StripImgMD   *regexp.Regexp
	StripImgHTML *regexp.Regexp
}

// PhaseSpec declares "what" a phase needs, not "how" the app calls it.
type PhaseSpec struct {
	// Human/LLM-facing metadata about the phase.
	Description string
	Consumes    []string
	Produces    []string
	UsesLLM     bool
	Tags        []string
	Metadata    map[string]string

	Key         string                                           // e.g. "m0"
	File        string                                           // e.g. "m0.json"
	BuildInput  func(ctx context.Context, env *Env) (any, error) // produce logical input
	Run         func(ctx context.Context, in any, env *Env) (any, error)
	Fingerprint func(in any, env *Env) string // stable hash for caching
	Downstream  []string                      // phases to invalidate when forced
	Requires    []string
	Strategy    CacheStrategy // how to cache (json, versioned, none)
}

// Descriptor converts a PhaseSpec into a plan.PhaseDescriptor with defensive copies.
func (spec PhaseSpec) Descriptor() plan.PhaseDescriptor {
	copySlice := func(in []string) []string {
		if len(in) == 0 {
			return nil
		}
		out := make([]string, len(in))
		copy(out, in)
		return out
	}
	copyMap := func(in map[string]string) map[string]string {
		if len(in) == 0 {
			return nil
		}
		out := make(map[string]string, len(in))
		for k, v := range in {
			out[k] = v
		}
		return out
	}

	return plan.PhaseDescriptor{
		Key:        spec.Key,
		Summary:    spec.Description,
		Consumes:   copySlice(spec.Consumes),
		Produces:   copySlice(spec.Produces),
		Requires:   copySlice(spec.Requires),
		Downstream: copySlice(spec.Downstream),
		UsesLLM:    spec.UsesLLM,
		Tags:       copySlice(spec.Tags),
		Metadata:   copyMap(spec.Metadata),
	}
}

// DescribeRegistry flattens a registry to descriptors for LLM planning.
func DescribeRegistry(reg map[string]PhaseSpec) []plan.PhaseDescriptor {
	out := make([]plan.PhaseDescriptor, 0, len(reg))
	for _, spec := range reg {
		out = append(out, spec.Descriptor())
	}
	return out
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
	fs := ensureFS(env.ArtifactFS)
	mp, op := s.metaPath(spec, env), s.outPath(spec, env)
	if !FileExists(fs, mp) || !FileExists(fs, op) {
		return nil, false
	}
	var m cacheMeta
	if b, err := fs.SafeReadFile(mp); err == nil && json.Unmarshal(b, &m) == nil {
		if m.Inputs == inputFP && m.Salt == env.ModelSalt {
			var out any
			if b, err := fs.SafeReadFile(op); err == nil && json.Unmarshal(b, &out) == nil {
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
	entries, _ := ensureFS(env.ArtifactFS).SafeReadDir(env.OutDir)
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
func ExecutePhase(ctx context.Context, spec PhaseSpec, env *Env) error {
	if len(spec.Requires) > 0 {
		visiting := make(map[string]bool)
		for _, r := range spec.Requires {
			if err := ensureArtifact(ctx, r, env, visiting); err != nil {
				return err
			}
		}
	}

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
	if env.ForceFrom != "" && env.ForceFrom == strings.ToLower(spec.Key) && env.Resolver != nil {
		for _, d := range spec.Downstream {
			if ds, ok := env.Resolver.Get(d); ok {
				_ = ds.Strategy.Invalidate(ctx, ds, env)
			}
		}
	}
	return nil
}

func ensureArtifact(ctx context.Context, key string, env *Env, visiting map[string]bool) error {
	if env == nil || env.Resolver == nil {
		return fmt.Errorf("runner: resolver is not configured")
	}
	if normalizeKey(key) == "" {
		return fmt.Errorf("runner: empty required phase key")
	}
	spec, ok := env.Resolver.Get(key)
	if !ok {
		fallback := filepath.Join(env.OutDir, normalizeKey(key)+".json")
		if FileExists(env.ArtifactFS, fallback) {
			return nil
		}
		return fmt.Errorf("runner: unknown required phase %s", key)
	}
	path := filepath.Join(env.OutDir, spec.File)
	if FileExists(env.ArtifactFS, path) {
		return nil
	}
	if visiting == nil {
		visiting = make(map[string]bool)
	}
	specKey := normalizeKey(spec.Key)
	if visiting[specKey] {
		return fmt.Errorf("runner: cyclic phase dependency detected at %s", spec.Key)
	}
	visiting[specKey] = true
	defer delete(visiting, specKey)
	for _, r := range spec.Requires {
		if err := ensureArtifact(ctx, r, env, visiting); err != nil {
			return err
		}
	}
	if err := ExecutePhase(ctx, spec, env); err != nil {
		return fmt.Errorf("failed to build required phase %s: %w", spec.Key, err)
	}
	return nil
}

// --------------------- Common helpers (hash, json, files) -------------------

func JSONFingerprint(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:])[:16]
}

func FileExists(fs *safeio.SafeFS, path string) bool {
	fs = ensureFS(fs)
	fi, err := fs.SafeStat(path)
	return err == nil && !fi.IsDir()
}

func ReadJSON(fs *safeio.SafeFS, dir, name string, v any) {
	fs = ensureFS(fs)
	b, err := fs.SafeReadFile(filepath.Join(dir, name))
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

func Artifact[T any](env *Env, key string) (T, error) {
	var zero T
	if env == nil {
		return zero, fmt.Errorf("runner: env is nil")
	}
	norm := normalizeKey(key)
	if norm == "" {
		return zero, fmt.Errorf("runner: empty phase key")
	}
	filename := norm + ".json"
	if env.Resolver != nil {
		if spec, ok := env.Resolver.Get(key); ok && strings.TrimSpace(spec.File) != "" {
			filename = spec.File
		}
	}
	fs := ensureFS(env.ArtifactFS)
	path := filepath.Join(env.OutDir, filename)
	b, err := fs.SafeReadFile(path)
	if err != nil {
		return zero, fmt.Errorf("runner: read artifact %s: %w", filename, err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return zero, fmt.Errorf("runner: decode artifact %s: %w", filename, err)
	}
	return out, nil
}

func MustArtifact[T any](env *Env, key string) T {
	v, err := Artifact[T](env, key)
	if err != nil {
		panic(err)
	}
	return v
}

func NextVersion(outDir, key string) int {
	entries, err := ensureFS(nil).SafeReadDir(outDir)
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

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func ensureFS(fs *safeio.SafeFS) *safeio.SafeFS {
	if fs != nil {
		return fs
	}
	if dfs := safeio.Default(); dfs != nil {
		return dfs
	}
	log.Fatal("safe filesystem is not configured")
	return nil
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
