package runner

import (
	"encoding/json"
	"fmt"
	"path/filepath"
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
	// Artifact loads a required worker output into target.
	// Returns error if the worker key is not declared in 'Requires'.
	Artifact(key string, target any) error

	// Repo returns the repository name.
	Repo() string

	// Root returns the repository root path.
	Root() string

	// Env exposes the raw environment for advanced usage (use sparingly).
	Env() Runtime
}

// depsImpl implements Deps and tracks accesses.
type depsImpl struct {
	runtime  Runtime
	requires map[string]bool
	accessed map[string]bool
	worker   string
}

func newDeps(runtime Runtime, worker string, requires []string) *depsImpl {
	reqMap := make(map[string]bool, len(requires))
	for _, r := range requires {
		reqMap[normalizeKey(r)] = true
	}
	return &depsImpl{
		runtime:  runtime,
		requires: reqMap,
		accessed: make(map[string]bool),
		worker:   worker,
	}
}

func (d *depsImpl) Artifact(key string, target any) error {
	norm := normalizeKey(key)
	if !d.requires[norm] {
		return fmt.Errorf("worker %q requested artifact %q but it is not declared in Requires", d.worker, key)
	}
	d.accessed[norm] = true

	artifacts := d.runtime.Artifacts()
	if artifacts == nil {
		return fmt.Errorf("artifact access is not configured")
	}
	artifactName := resolveArtifactName(d.runtime, key)
	b, err := artifacts.Read(artifactName)
	if err != nil {
		return fmt.Errorf("read artifact %s: %w", key, err)
	}
	if err := json.Unmarshal(b, target); err != nil {
		return fmt.Errorf("decode artifact %s: %w", key, err)
	}
	return nil
}

func (d *depsImpl) Repo() string {
	if d.runtime == nil || d.runtime.GetRepoFS() == nil {
		return ""
	}
	root := filepath.Clean(d.runtime.GetRepoFS().Root())
	if root == "." || root == "/" {
		return ""
	}
	return filepath.Base(root)
}

func (d *depsImpl) Root() string {
	if d.runtime == nil || d.runtime.GetRepoFS() == nil {
		return ""
	}
	return d.runtime.GetRepoFS().Root()
}

func (d *depsImpl) Env() Runtime {
	return d.runtime
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

func resolveArtifactName(runtime Runtime, key string) string {
	artifactKey := normalizeKey(key)
	if runtime != nil && runtime.GetResolver() != nil {
		if spec, ok := runtime.GetResolver().Get(key); ok {
			if v := strings.TrimSpace(spec.Key); v != "" {
				artifactKey = v
			}
		}
	}
	return artifactKey + ".json"
}
