package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// --------------------- JSON file strategy ---------------------

type jsonStrategy struct{}

// JSONStrategy returns the standard JSON caching strategy.
func JSONStrategy() CacheStrategy { return jsonStrategy{} }

type cacheMeta struct {
	Inputs    string    `json:"inputs"`
	Salt      string    `json:"salt,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (s jsonStrategy) TryLoad(ctx context.Context, spec WorkerSpec, runtime Runtime, inputFP string) (WorkerOutput, bool) {
	var zero WorkerOutput
	if runtime.GetForceFrom() != "" && runtime.GetForceFrom() == strings.ToLower(spec.Key) {
		return zero, false
	}
	fs := ensureFS(runtime.GetArtifactFS())
	mp := filepath.Join(runtime.GetOutDir(), spec.Key+".meta.json")
	op := filepath.Join(runtime.GetOutDir(), spec.Key+".json")
	if !FileExists(fs, mp) || !FileExists(fs, op) {
		return zero, false
	}
	var m cacheMeta
	if b, err := fs.SafeReadFile(mp); err == nil && json.Unmarshal(b, &m) == nil {
		if m.Inputs == inputFP && m.Salt == runtime.GetModelSalt() {
			var out any
			if b, err := fs.SafeReadFile(op); err == nil && json.Unmarshal(b, &out) == nil {
				log.Printf("%s: using cache → %s", strings.ToUpper(spec.Key), op)
				return WorkerOutput{RuntimeState: out, ClientView: nil}, true
			}
		}
	}
	return zero, false
}

func (s jsonStrategy) Save(ctx context.Context, spec WorkerSpec, runtime Runtime, out WorkerOutput, inputFP string) error {
	_ = os.MkdirAll(runtime.GetOutDir(), 0755)
	mp := filepath.Join(runtime.GetOutDir(), spec.Key+".meta.json")
	op := filepath.Join(runtime.GetOutDir(), spec.Key+".json")
	if b, e := json.MarshalIndent(out.RuntimeState, "", "  "); e == nil {
		_ = os.WriteFile(op, b, 0o644)
	}
	mb, _ := json.MarshalIndent(cacheMeta{Inputs: inputFP, Salt: runtime.GetModelSalt(), CreatedAt: time.Now()}, "", "  ")
	_ = os.WriteFile(mp, mb, 0o644)
	log.Printf("%s → %s", strings.ToUpper(spec.Key), op)
	return nil
}

func (s jsonStrategy) Invalidate(ctx context.Context, spec WorkerSpec, runtime Runtime) error {
	_ = os.Remove(filepath.Join(runtime.GetOutDir(), spec.Key+".json"))
	_ = os.Remove(filepath.Join(runtime.GetOutDir(), spec.Key+".meta.json"))
	return nil
}

// --------------------- Versioned JSON strategy -------------------------

// versionedStrategy always writes a new versioned file and updates latest.
// Cache read is intentionally disabled (exploratory / append-only).
type versionedStrategy struct{}

// VersionedStrategy returns the versioned (no-cache) strategy.
func VersionedStrategy() CacheStrategy { return versionedStrategy{} }

func (versionedStrategy) TryLoad(ctx context.Context, spec WorkerSpec, runtime Runtime, inputFP string) (WorkerOutput, bool) {
	// Never reuse cache for versioned workers.
	return WorkerOutput{}, false
}

func (versionedStrategy) Save(ctx context.Context, spec WorkerSpec, runtime Runtime, out WorkerOutput, inputFP string) error {
	// Always start at v1 for each run; overwrite v1 and latest, and optionally prune older versions.
	versioned := fmt.Sprintf("%s_v1.json", spec.Key)
	versionedPath := filepath.Join(runtime.GetOutDir(), versioned)
	latestPath := filepath.Join(runtime.GetOutDir(), spec.Key+".json")

	if b, e := json.MarshalIndent(out.RuntimeState, "", "  "); e == nil {
		_ = os.WriteFile(versionedPath, b, 0o644)
		_ = os.WriteFile(latestPath, b, 0o644)
	}
	// meta is optional for versioned write; record last inputs for debugging
	mp := filepath.Join(runtime.GetOutDir(), spec.Key+".meta.json")
	mb, _ := json.MarshalIndent(cacheMeta{Inputs: inputFP, Salt: runtime.GetModelSalt(), CreatedAt: time.Now()}, "", "  ")
	_ = os.WriteFile(mp, mb, 0o644)

	// Best-effort pruning of other versions
	entries, _ := ensureFS(runtime.GetArtifactFS()).SafeReadDir(runtime.GetOutDir())
	re := regexp.MustCompile(fmt.Sprintf(`^%s_v(\d+)\.json$`, regexp.QuoteMeta(spec.Key)))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if m := re.FindStringSubmatch(name); len(m) == 2 && name != versioned {
			_ = os.Remove(filepath.Join(runtime.GetOutDir(), name))
		}
	}
	log.Printf("%s → %s (reset to v1; updated %s)", strings.ToUpper(spec.Key), versionedPath, latestPath)
	return nil
}

func (versionedStrategy) Invalidate(ctx context.Context, spec WorkerSpec, runtime Runtime) error {
	// No-op: keeps versions; do not delete history. Keep latest key artifact as well.
	return nil
}
