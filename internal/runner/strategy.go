package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	artifacts := runtime.Artifacts()
	if artifacts == nil {
		return zero, false
	}
	metaName := spec.Key + ".meta.json"
	outName := spec.Key + ".json"
	mb, err := artifacts.Read(ctx, metaName)
	if err != nil {
		return zero, false
	}
	ob, err := artifacts.Read(ctx, outName)
	if err != nil {
		return zero, false
	}
	var m cacheMeta
	if json.Unmarshal(mb, &m) == nil && m.Inputs == inputFP && m.Salt == runtime.GetModelSalt() {
		var out any
		if json.Unmarshal(ob, &out) == nil {
			log.Printf("%s: using cache → %s", strings.ToUpper(spec.Key), outName)
			return WorkerOutput{RuntimeState: out, ClientView: nil}, true
		}
	}
	return zero, false
}

func (s jsonStrategy) Save(ctx context.Context, spec WorkerSpec, runtime Runtime, out WorkerOutput, inputFP string) error {
	artifacts := runtime.Artifacts()
	if artifacts == nil {
		return fmt.Errorf("artifact access is nil")
	}
	metaName := spec.Key + ".meta.json"
	outName := spec.Key + ".json"
	if b, e := json.MarshalIndent(out.RuntimeState, "", "  "); e == nil {
		_ = artifacts.Write(ctx, outName, b)
	}
	mb, _ := json.MarshalIndent(cacheMeta{Inputs: inputFP, Salt: runtime.GetModelSalt(), CreatedAt: time.Now()}, "", "  ")
	_ = artifacts.Write(ctx, metaName, mb)
	log.Printf("%s → %s", strings.ToUpper(spec.Key), outName)
	return nil
}

func (s jsonStrategy) Invalidate(ctx context.Context, spec WorkerSpec, runtime Runtime) error {
	artifacts := runtime.Artifacts()
	if artifacts == nil {
		return nil
	}
	_ = artifacts.Remove(ctx, spec.Key+".json")
	_ = artifacts.Remove(ctx, spec.Key+".meta.json")
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
	latest := spec.Key + ".json"
	artifacts := runtime.Artifacts()
	if artifacts == nil {
		return fmt.Errorf("artifact access is nil")
	}

	if b, e := json.MarshalIndent(out.RuntimeState, "", "  "); e == nil {
		_ = artifacts.Write(ctx, versioned, b)
		_ = artifacts.Write(ctx, latest, b)
	}
	// meta is optional for versioned write; record last inputs for debugging
	metaName := spec.Key + ".meta.json"
	mb, _ := json.MarshalIndent(cacheMeta{Inputs: inputFP, Salt: runtime.GetModelSalt(), CreatedAt: time.Now()}, "", "  ")
	_ = artifacts.Write(ctx, metaName, mb)

	// Best-effort pruning of other versions
	entries, _ := artifacts.List(ctx)
	re := regexp.MustCompile(fmt.Sprintf(`^%s_v(\d+)\.json$`, regexp.QuoteMeta(spec.Key)))
	for _, name := range entries {
		if m := re.FindStringSubmatch(name); len(m) == 2 && name != versioned {
			_ = artifacts.Remove(ctx, name)
		}
	}
	log.Printf("%s → %s (reset to v1; updated %s)", strings.ToUpper(spec.Key), versioned, latest)
	return nil
}

func (versionedStrategy) Invalidate(ctx context.Context, spec WorkerSpec, runtime Runtime) error {
	// No-op: keeps versions; do not delete history. Keep latest key artifact as well.
	return nil
}
