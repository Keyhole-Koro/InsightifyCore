package runner

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// DepsUsageMode controls how strictly to enforce declared dependency usage.
type DepsUsageMode int

const (
	DepsUsageError  DepsUsageMode = iota // default: treat unused Requires as errors
	DepsUsageWarn                        // log warnings for unused Requires
	DepsUsageIgnore                      // skip unused Requires checks
)

// Deps controls access to dependencies during BuildInput.
// It enforces that requested artifacts are declared in 'Requires'
// and tracks usage to detect unused declarations.
type Deps interface {
	// Artifact loads a required phase output into target.
	// Returns error if the phase key is not declared in 'Requires'.
	Artifact(key string, target any) error

	// Repo returns the repository name.
	Repo() string

	// Root returns the repository root path.
	Root() string

	// Env exposes the raw environment for advanced usage (use sparingly).
	Env() *Env
}

// depsImpl implements Deps and tracks accesses.
type depsImpl struct {
	env      *Env
	requires map[string]bool
	accessed map[string]bool
	phase    string
}

func newDeps(env *Env, phase string, requires []string) *depsImpl {
	reqMap := make(map[string]bool, len(requires))
	for _, r := range requires {
		reqMap[normalizeKey(r)] = true
	}
	return &depsImpl{
		env:      env,
		requires: reqMap,
		accessed: make(map[string]bool),
		phase:    phase,
	}
}

func (d *depsImpl) Artifact(key string, target any) error {
	norm := normalizeKey(key)
	if !d.requires[norm] {
		return fmt.Errorf("phase %q requested artifact %q but it is not declared in Requires", d.phase, key)
	}
	d.accessed[norm] = true

	// Load artifact logic (similar to original Artifact generic function)
	filename := norm + ".json"
	if d.env.Resolver != nil {
		if spec, ok := d.env.Resolver.Get(key); ok && strings.TrimSpace(spec.File) != "" {
			filename = spec.File
		}
	}
	fs := ensureFS(d.env.ArtifactFS)
	// assuming artifact paths are in Env.OutDir
	// Note: We need to use d.env.OutDir but currently runner/util.go imports safeio directly.
	// We'll reuse the logic from Artifact helper but adapted here to avoid circular imports or copy-paste.
	// Actually, let's just use the existing Artifact helper logic but injected.
	// For now, simple read:
	path := d.env.OutDir + "/" + filename // simplistic join
	if fs != nil {
		// Use safeio properly
		// But wait, Artifact() function in registry.go handles this.
		// Let's call the env's helper if possible or duplicate the logic safely.
		// Reusing Artifact[T] generic requires T, but here we have 'any'.
		// We have to unmarshal into target.
		b, err := fs.SafeReadFile(path)
		if err != nil {
			return fmt.Errorf("read artifact %s: %w", filename, err)
		}
		if err := json.Unmarshal(b, target); err != nil {
			return fmt.Errorf("decode artifact %s: %w", filename, err)
		}
		return nil
	}
	return fmt.Errorf("fs not configured")
}

func (d *depsImpl) Repo() string {
	return d.env.Repo
}

func (d *depsImpl) Root() string {
	return d.env.RepoRoot
}

func (d *depsImpl) Env() *Env {
	return d.env
}

// verifyUsage checks for over-fetching (declared but unused).
func (d *depsImpl) verifyUsage() []string {
	var unused []string
	for req := range d.requires {
		if !d.accessed[req] {
			unused = append(unused, req)
		}
	}
	sort.Strings(unused)
	return unused
}
