package runner

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"insightify/internal/safeio"
	t "insightify/internal/types"
)

// Common helpers (hash, json, files).

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
		if m := re.FindStringSubmatch(e.Name()); len(m) == 2 {
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

// Utility transforms used in several phases.

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
		if b := filepath.Base(filepath.ToSlash(p)); b != "" {
			out = append(out, b)
		}
	}
	return out
}
