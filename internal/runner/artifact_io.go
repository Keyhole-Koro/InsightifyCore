package runner

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"insightify/internal/safeio"
)

// Artifact loads a worker artifact from disk into the target type.
func Artifact[T any](runtime Runtime, key string) (T, error) {
	var zero T
	if runtime == nil {
		return zero, fmt.Errorf("runner: runtime is nil")
	}
	norm := normalizeKey(key)
	if norm == "" {
		return zero, fmt.Errorf("runner: empty worker key")
	}
	fs := ensureFS(runtime.GetArtifactFS())
	path, label, err := resolveArtifactPath(runtime, key)
	if err != nil {
		return zero, err
	}
	b, err := fs.SafeReadFile(path)
	if err != nil {
		return zero, fmt.Errorf("runner: read artifact %s: %w", label, err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return zero, fmt.Errorf("runner: decode artifact %s: %w", label, err)
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

func resolveArtifactPath(runtime Runtime, key string) (string, string, error) {
	if runtime == nil {
		return "", "", fmt.Errorf("runner: runtime is nil")
	}
	norm := normalizeKey(key)
	if norm == "" {
		return "", "", fmt.Errorf("runner: empty worker key")
	}
	filename := norm + ".json"
	var specKey string
	if runtime.GetResolver() != nil {
		if s, ok := runtime.GetResolver().Get(key); ok {
			specKey = strings.TrimSpace(s.Key)
		}
	}
	if specKey != "" {
		filename = specKey + ".json"
	}
	fs := ensureFS(runtime.GetArtifactFS())

	primary := filepath.Join(runtime.GetOutDir(), filename)
	if FileExists(fs, primary) {
		return primary, filepath.Base(primary), nil
	}
	// Backward-compat: pre-key migration JSON artifacts were stored at <OutDir>/<key>/output.json.
	if specKey != "" {
		legacy := filepath.Join(runtime.GetOutDir(), specKey, "output.json")
		if FileExists(fs, legacy) {
			return legacy, filepath.Base(legacy), nil
		}
	}
	return primary, filepath.Base(primary), nil
}
