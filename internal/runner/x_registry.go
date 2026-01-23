package runner

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"insightify/internal/llm"
	extpipe "insightify/internal/pipeline/external"
	"insightify/internal/safeio"
	t "insightify/internal/types"
	cb "insightify/internal/types/codebase"
	ex "insightify/internal/types/external"
	ml "insightify/internal/types/mainline"
)

// BuildRegistryExternal wires the external (x*) pipeline stages.
func BuildRegistryExternal(env *Env) map[string]PhaseSpec {
	reg := map[string]PhaseSpec{}

	reg["x0"] = PhaseSpec{
		Key:         "x0",
		File:        "x0.json",
		Requires:    []string{"m1", "c4"},
		Description: "LLM summarizes external systems/infra using architecture (m1) + identifier refs (c4), surfacing evidence gaps.",
		Consumes:    []string{"architecture_hypothesis", "references", "layout_roots"},
		Produces:    []string{"external_overview", "evidence_gaps"},
		UsesLLM:     true,
		Tags:        []string{"external", "infra"},
		BuildInput: func(ctx context.Context, env *Env) (any, error) {
			m0, err := Artifact[ml.M0Out](env, "m0")
			if err != nil {
				return nil, err
			}
			m1, err := Artifact[ml.M1Out](env, "m1")
			if err != nil {
				return nil, err
			}
			c4, err := Artifact[cb.C4Out](env, "c4")
			if err != nil {
				return nil, err
			}
			samples := collectInfraSamples(env.RepoFS, env.RepoRoot, m0, 16, 16000)
			summaries := selectIdentifierSummaries(c4.Files, env.RepoRoot, m0, 40)
			return ex.X0In{
				Repo:                env.Repo,
				Roots:               m0,
				Architecture:        m1,
				ConfigSamples:       samples,
				IdentifierSummaries: summaries,
				ConfidenceThreshold: 0.65,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (any, error) {
			ctx = llm.WithPhase(ctx, "x0")
			p := extpipe.X0{LLM: env.LLM}
			out, err := p.Run(ctx, in.(ex.X0In))
			if err != nil {
				return nil, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   ex.X0In
				Salt string
			}{in.(ex.X0In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["x1"] = PhaseSpec{
		Key:         "x1",
		File:        "x1.json",
		Requires:    []string{"x0"},
		Description: "LLM drills into evidence gaps from x0 by opening targeted files/snippets.",
		Consumes:    []string{"external_overview", "evidence_gaps"},
		Produces:    []string{"external_verification"},
		UsesLLM:     true,
		Tags:        []string{"external", "infra"},
		BuildInput: func(ctx context.Context, env *Env) (any, error) {
			prev, err := Artifact[ex.X0Out](env, "x0")
			if err != nil {
				return nil, err
			}
			files := collectGapFiles(env.RepoFS, env.RepoRoot, prev.EvidenceGaps, 24, 64000)
			return ex.X1In{
				Repo:     env.Repo,
				Previous: prev,
				Files:    files,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (any, error) {
			ctx = llm.WithPhase(ctx, "x1")
			p := extpipe.X1{LLM: env.LLM}
			out, err := p.Run(ctx, in.(ex.X1In))
			if err != nil {
				return nil, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   ex.X1In
				Salt string
			}{in.(ex.X1In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	return reg
}

func collectInfraSamples(fs *safeio.SafeFS, repoRoot string, roots ml.M0Out, maxFiles, maxBytes int) []t.OpenedFile {
	if fs == nil || maxFiles <= 0 {
		return nil
	}
	candidates := make([]string, 0, maxFiles*3)
	seen := make(map[string]struct{})
	for _, f := range append(append([]string{}, roots.ConfigFiles...), roots.RuntimeConfigFiles...) {
		appendCandidate(&candidates, seen, f)
	}
	rootDirs := append(append([]string{}, roots.ConfigRoots...), roots.RuntimeConfigRoots...)
	rootDirs = append(rootDirs, roots.BuildRoots...)
	for _, dir := range uniqueStrings(rootDirs...) {
		gatherInfraDir(fs, dir, 0, maxFiles*4, &candidates, seen)
		if len(candidates) >= maxFiles*4 {
			break
		}
	}

	sort.Strings(candidates)
	if len(candidates) > maxFiles {
		candidates = candidates[:maxFiles]
	}

	var samples []t.OpenedFile
	for _, path := range candidates {
		of, err := readFileSample(fs, repoRoot, path, maxBytes)
		if err != nil {
			continue
		}
		samples = append(samples, of)
		if len(samples) >= maxFiles {
			break
		}
	}
	return samples
}

func gatherInfraDir(fs *safeio.SafeFS, dir string, depth, limit int, dest *[]string, seen map[string]struct{}) {
	if fs == nil || dir == "" || depth > 2 || len(*dest) >= limit {
		return
	}
	dirPath := normalizeCandidatePath(dir)
	entries, err := fs.SafeReadDir(toFSPath(dirPath))
	if err != nil {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	processed := 0
	for _, entry := range entries {
		if processed >= 50 {
			break
		}
		processed++
		name := entry.Name()
		child := filepath.Join(dirPath, name)
		if entry.IsDir() {
			if depth < 1 || looksInfraDir(name) {
				gatherInfraDir(fs, child, depth+1, limit, dest, seen)
			}
			continue
		}
		if isInfraFile(name) {
			appendCandidate(dest, seen, child)
			if len(*dest) >= limit {
				return
			}
		}
	}
}

func readFileSample(fs *safeio.SafeFS, repoRoot, path string, maxBytes int) (t.OpenedFile, error) {
	if fs == nil {
		return t.OpenedFile{}, fmt.Errorf("repo filesystem is nil")
	}
	f, err := fs.SafeOpen(toFSPath(path))
	if err != nil {
		return t.OpenedFile{}, err
	}
	defer f.Close()
	var reader io.Reader = f
	if maxBytes > 0 {
		reader = io.LimitReader(f, int64(maxBytes))
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return t.OpenedFile{}, err
	}
	rel := normalizeRepoPath(repoRoot, f.Name(), path)
	return t.OpenedFile{Path: rel, Content: string(data)}, nil
}

func selectIdentifierSummaries(reports []cb.IdentifierReport, repoRoot string, roots ml.M0Out, max int) []ex.IdentifierSummary {
	if max <= 0 {
		return nil
	}
	var (
		targetPrefixes = buildPrefixSet(repoRoot, append(append([]string{}, roots.ConfigRoots...), roots.RuntimeConfigRoots...), roots.BuildRoots)
		priority       []ex.IdentifierSummary
		fallback       []ex.IdentifierSummary
	)
	for _, rep := range reports {
		path := filepath.ToSlash(rep.Path)
		inInfra := hasAnyPrefix(path, targetPrefixes)
		for _, sig := range rep.Identifiers {
			snap := ex.IdentifierSummary{
				Path:     path,
				Name:     sig.Name,
				Role:     sig.Role,
				Summary:  truncateString(sig.Summary, 480),
				Lines:    sig.Lines,
				Scope:    sig.Scope,
				Requires: sig.Requires,
				Source:   "c4",
			}
			if len(rep.Notes) > 0 {
				snap.Notes = append([]string(nil), rep.Notes...)
			}
			if inInfra || usesExternalRequirement(sig.Requires) {
				priority = append(priority, snap)
			} else if len(fallback) < max {
				fallback = append(fallback, snap)
			}
			if len(priority) >= max {
				break
			}
		}
		if len(priority) >= max {
			break
		}
	}
	if len(priority) > max {
		priority = priority[:max]
	}
	if len(priority) < max {
		need := max - len(priority)
		if need > len(fallback) {
			need = len(fallback)
		}
		priority = append(priority, fallback[:need]...)
	}
	return priority
}

func collectGapFiles(fs *safeio.SafeFS, repoRoot string, gaps []ex.EvidenceGap, maxFiles, maxBytes int) []t.OpenedFile {
	if fs == nil || maxFiles <= 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var samples []t.OpenedFile
	for _, gap := range gaps {
		for _, suggestion := range gap.Suggested {
			if !isFileLikeSuggestion(suggestion.Kind) {
				continue
			}
			path := normalizeCandidatePath(suggestion.Path)
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			of, err := readFileSample(fs, repoRoot, path, maxBytes)
			if err != nil {
				continue
			}
			seen[path] = struct{}{}
			samples = append(samples, of)
			if len(samples) >= maxFiles {
				return samples
			}
		}
	}
	return samples
}

func isFileLikeSuggestion(kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "", "file", "config", "doc", "document":
		return true
	default:
		return false
	}
}

func usesExternalRequirement(reqs []cb.IdentifierRequirement) bool {
	for _, r := range reqs {
		switch strings.ToLower(r.Origin) {
		case "", "user":
			continue
		default:
			return true
		}
	}
	return false
}

func buildPrefixSet(repoRoot string, groups ...[]string) []string {
	var prefixes []string
	for _, group := range groups {
		for _, p := range group {
			norm := normalizeRepoPath(repoRoot, p, p)
			if norm == "" || norm == "." {
				continue
			}
			if strings.HasSuffix(norm, "/") {
				prefixes = append(prefixes, norm)
				continue
			}
			prefixes = append(prefixes, norm, norm+"/")
		}
	}
	sort.Strings(prefixes)
	return uniqueStrings(prefixes...)
}

func hasAnyPrefix(path string, prefixes []string) bool {
	for _, pre := range prefixes {
		if strings.HasPrefix(path, pre) {
			return true
		}
	}
	return false
}

func normalizeCandidatePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func normalizeRepoPath(repoRoot, absPath, fallback string) string {
	out := strings.TrimSpace(absPath)
	if out == "" {
		out = strings.TrimSpace(fallback)
	}
	if repoRoot != "" {
		target := out
		if !filepath.IsAbs(target) {
			target = filepath.Join(repoRoot, target)
		}
		if rel, err := filepath.Rel(repoRoot, target); err == nil {
			out = rel
		}
	}
	out = filepath.ToSlash(filepath.Clean(out))
	return out
}

func appendCandidate(dest *[]string, seen map[string]struct{}, path string) {
	path = normalizeCandidatePath(path)
	if path == "" {
		return
	}
	if _, ok := seen[path]; ok {
		return
	}
	seen[path] = struct{}{}
	*dest = append(*dest, path)
}

func toFSPath(path string) string {
	if path == "" {
		return path
	}
	return filepath.FromSlash(path)
}

func truncateString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

var infraExts = map[string]struct{}{
	".tf":     {},
	".tfvars": {},
	".hcl":    {},
	".yaml":   {},
	".yml":    {},
	".json":   {},
	".toml":   {},
	".ini":    {},
	".sh":     {},
	".ps1":    {},
	".bicep":  {},
	".sql":    {},
}

var infraExactFiles = map[string]struct{}{
	"Dockerfile":          {},
	"docker-compose.yml":  {},
	"docker-compose.yaml": {},
	"template.yml":        {},
	"template.yaml":       {},
	"samconfig.toml":      {},
	"Makefile":            {},
	"package.json":        {},
	"package-lock.json":   {},
	"pnpm-lock.yaml":      {},
}

func isInfraFile(name string) bool {
	if _, ok := infraExactFiles[name]; ok {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	if _, ok := infraExts[ext]; ok {
		return true
	}
	return false
}

func looksInfraDir(name string) bool {
	name = strings.ToLower(name)
	keywords := []string{
		"infra", "infrastructure", "deploy", "deployment", "terraform", "iac",
		"cloud", "aws", "gcp", "azure", "ops", "devops", "config", "scripts",
		"pipelines", "ci", "cd", "build", "sam", "serverless",
	}
	for _, kw := range keywords {
		if strings.Contains(name, kw) {
			return true
		}
	}
	return false
}

func uniqueStrings(values ...string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
