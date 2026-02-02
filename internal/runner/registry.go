package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"insightify/internal/artifact"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/mcp"
	"insightify/internal/safeio"
	"insightify/internal/wordidx"
)

// SpecResolver resolves phase keys to specs, enabling cross-registry lookup.
type SpecResolver interface {
	Get(key string) (PhaseSpec, bool)
	List() []PhaseSpec
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

// List returns all registered phase specs.
func (r MapResolver) List() []PhaseSpec {
	specs := make([]PhaseSpec, 0, len(r.specs))
	for _, s := range r.specs {
		specs = append(specs, s)
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Key < specs[j].Key })
	return specs
}

// MergeRegistries flattens multiple phase registries into a single resolver.
// It also computes downstream dependencies automatically from 'Requires'.
func MergeRegistries(regs ...map[string]PhaseSpec) SpecResolver {
	merged := make(map[string]PhaseSpec, 16)
	downstream := make(map[string][]string)

	for _, reg := range regs {
		for k, v := range reg {
			nk := normalizeKey(k)
			merged[nk] = v
			for _, req := range v.Requires {
				nr := normalizeKey(req)
				downstream[nr] = append(downstream[nr], nk)
			}
		}
	}

	// Update downstream fields in specs
	for k, v := range merged {
		if ds, ok := downstream[k]; ok {
			// Sort for determinism
			sort.Strings(ds)
			v.Downstream = ds
			merged[k] = v
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

	MCP     *mcp.Registry
	MCPHost mcp.Host

	ModelSalt string
	ForceFrom string
	DepsUsage DepsUsageMode

	LLM llmclient.LLMClient

	WordIndexer wordidx.AggIndex

	Index  []artifact.FileIndexEntry
	MDDocs []artifact.MDDoc
}

// PhaseOutput bundles internal RuntimeState with an optional ClientView payload for the client.
type PhaseOutput struct {
	RuntimeState any
	ClientView   any
}

// PhaseSpec declares "what" a phase needs, not "how" the app calls it.
type PhaseSpec struct {
	Description string // ログやエラーメッセージ用の最小限の説明

	Key         string                                            // e.g. "m0"
	File        string                                            // e.g. "m0.json"
	BuildInput  func(ctx context.Context, deps Deps) (any, error) // produce logical input
	Run         func(ctx context.Context, in any, env *Env) (PhaseOutput, error)
	Fingerprint func(in any, env *Env) string // stable hash for caching
	Downstream  []string                      // automatically computed
	Requires    []string
	Strategy    CacheStrategy // how to cache (json, versioned, none)
}

// CacheStrategy abstracts artifact persistence policies (json, versioned, …).
type CacheStrategy interface {
	// TryLoad returns (out, true) if cache hit and not forced.
	TryLoad(ctx context.Context, spec PhaseSpec, env *Env, inputFP string) (PhaseOutput, bool)
	// Save persists result and metadata.
	Save(ctx context.Context, spec PhaseSpec, env *Env, out PhaseOutput, inputFP string) error
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

func (s jsonStrategy) dir(spec PhaseSpec, env *Env) string {
	return filepath.Join(env.OutDir, spec.Key)
}
func (s jsonStrategy) metaPath(spec PhaseSpec, env *Env) string {
	return filepath.Join(s.dir(spec, env), "meta.json")
}
func (s jsonStrategy) outPath(spec PhaseSpec, env *Env) string {
	return filepath.Join(s.dir(spec, env), "output.json")
}

func (s jsonStrategy) TryLoad(ctx context.Context, spec PhaseSpec, env *Env, inputFP string) (PhaseOutput, bool) {
	var zero PhaseOutput
	if env.ForceFrom != "" && env.ForceFrom == strings.ToLower(spec.Key) {
		return zero, false
	}
	fs := ensureFS(env.ArtifactFS)
	mp, op := s.metaPath(spec, env), s.outPath(spec, env)
	if !FileExists(fs, mp) || !FileExists(fs, op) {
		return zero, false
	}
	var m cacheMeta
	if b, err := fs.SafeReadFile(mp); err == nil && json.Unmarshal(b, &m) == nil {
		if m.Inputs == inputFP && m.Salt == env.ModelSalt {
			var out any
			if b, err := fs.SafeReadFile(op); err == nil && json.Unmarshal(b, &out) == nil {
				log.Printf("%s: using cache → %s", strings.ToUpper(spec.Key), op)
				return PhaseOutput{RuntimeState: out, ClientView: nil}, true
			}
		}
	}
	return zero, false
}

func (s jsonStrategy) Save(ctx context.Context, spec PhaseSpec, env *Env, out PhaseOutput, inputFP string) error {
	dir := s.dir(spec, env)
	_ = os.MkdirAll(dir, 0755)
	mp, op := s.metaPath(spec, env), s.outPath(spec, env)
	if b, e := json.MarshalIndent(out.RuntimeState, "", "  "); e == nil {
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

func (versionedStrategy) TryLoad(ctx context.Context, spec PhaseSpec, env *Env, inputFP string) (PhaseOutput, bool) {
	// Never reuse cache for versioned phases (consistent with previous x0 behavior).
	return PhaseOutput{}, false
}

func (versionedStrategy) Save(ctx context.Context, spec PhaseSpec, env *Env, out PhaseOutput, inputFP string) error {
	// Always start at v1 for each run; overwrite v1 and latest, and optionally prune older versions.
	versioned := fmt.Sprintf("%s_v1.json", spec.Key)
	versionedPath := filepath.Join(env.OutDir, versioned)
	latestPath := filepath.Join(env.OutDir, spec.File)

	if b, e := json.MarshalIndent(out.RuntimeState, "", "  "); e == nil {
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
	_, err := ExecutePhaseWithResult(ctx, spec, env)
	return err
}

// ExecutePhaseWithResult is the same as ExecutePhase but also returns PhaseOutput.
// RuntimeState is populated from the legacy Run() return; ClientView is nil unless a phase chooses to set it.
func ExecutePhaseWithResult(ctx context.Context, spec PhaseSpec, env *Env) (PhaseOutput, error) {
	var zero PhaseOutput
	if len(spec.Requires) > 0 {
		visiting := make(map[string]bool)
		for _, r := range spec.Requires {
			if err := ensureArtifact(ctx, r, env, visiting); err != nil {
				return zero, err
			}
		}
	}

	// Prepare Deps for usage tracking
	deps := newDeps(env, spec.Key, spec.Requires)

	// Build logical input using Deps
	in, err := spec.BuildInput(ctx, deps)
	if err != nil {
		return zero, err
	}

	// Verify usage (optional warning for now)
	if unused := deps.verifyUsage(); len(unused) > 0 {
		switch env.DepsUsage {
		case DepsUsageIgnore:
			// no-op
		case DepsUsageWarn:
			log.Printf("WARNING: phase %s declared but did not use: %v", spec.Key, unused)
		default:
			return zero, fmt.Errorf("phase %s declared but did not use: %v", spec.Key, unused)
		}
	}

	// Compute fingerprint
	fp := spec.Fingerprint(in, env)

	// Try cache (if strategy supports it)
	if out, ok := spec.Strategy.TryLoad(ctx, spec, env, fp); ok {
		return out, nil
	}

	// Run phase
	out, err := spec.Run(ctx, in, env)
	if err != nil {
		return zero, err
	}

	// Persist artifact via strategy (only RuntimeState should be cached)
	if err := spec.Strategy.Save(ctx, spec, env, out, fp); err != nil {
		return zero, err
	}

	// If forced, invalidate downstream caches (json-strategy only).
	if env.ForceFrom != "" && env.ForceFrom == strings.ToLower(spec.Key) && env.Resolver != nil {
		for _, d := range spec.Downstream {
			if ds, ok := env.Resolver.Get(d); ok {
				_ = ds.Strategy.Invalidate(ctx, ds, env)
			}
		}
	}
	return out, nil
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
