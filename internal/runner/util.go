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
