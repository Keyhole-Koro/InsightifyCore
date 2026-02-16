package runner

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"insightify/internal/safeio"
)

// Artifact loads a worker artifact from disk into the target type.
func Artifact[T any](runtime Runtime, key string) (T, error) {
	var zero T
	if runtime == nil {
		return zero, fmt.Errorf("runner: runtime is nil")
	}
	artifacts := runtime.Artifacts()
	if artifacts == nil {
		return zero, fmt.Errorf("runner: artifact access is nil")
	}
	b, err := artifacts.ReadWorker(key)
	if err != nil {
		return zero, fmt.Errorf("runner: read artifact %s: %w", key, err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return zero, fmt.Errorf("runner: decode artifact %s: %w", key, err)
	}
	return out, nil
}

// MustArtifact loads an artifact or panics.
func MustArtifact[T any](runtime Runtime, key string) T {
	v, err := Artifact[T](runtime, key)
	if err != nil {
		panic(err)
	}
	return v
}

// ReadJSON reads and unmarshals a JSON file from a directory.
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

// WriteJSON writes a value as indented JSON to a file.
func WriteJSON(dir, name string, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, name), b, 0o644)
}

// NextVersion finds the next available version number for a versioned artifact.
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
