package pipeline

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"insightify/internal/scan"
	t "insightify/internal/types"
)

// X1 builds a dependency graph from extractor specs (X0 output) by scanning files.
// This pipeline does not use an LLM — it is entirely programmatic.
type X1 struct{}

func (X1) Run(in t.X1In) (t.X1Out, error) {
	// Build ext -> compiled rules
	type rule struct {
		re       *regexp.Regexp
		modGroup int // capture index for module; defaults to 1
		spec     t.X0Spec
	}
	rulesByExt := map[string][]rule{}
	for _, s := range in.Specs {
		// Determine module capture index per rule
		modIdx := 1
		for _, r := range s.Rules {
			if g, ok := r.Captures["module"]; ok {
				modIdx = g
			} else {
				modIdx = 1
			}
			// compile
			re, err := regexp.Compile(r.Pattern)
			if err != nil {
				continue // skip invalid
			}
			rulesByExt[strings.ToLower(s.Ext)] = append(rulesByExt[strings.ToLower(s.Ext)], rule{re: re, modGroup: modIdx, spec: s})
		}
	}

	// Build allowed set from provided index (optional) and for existence checks
	filesSet := map[string]struct{}{}
	for _, f := range in.Index {
		filesSet[filepath.ToSlash(f.Path)] = struct{}{}
	}

	out := t.X1Out{}

	// Use scan.ScanWithOptions to traverse repo files; honor provided index when non-empty
	_ = scan.ScanWithOptions(in.Repo, scan.Options{}, func(fv scan.FileVisit) {
		if fv.IsDir {
			return
		}
		// If an index was provided, restrict processing to those files
		if len(filesSet) > 0 {
			if _, ok := filesSet[filepath.ToSlash(fv.Path)]; !ok {
				return
			}
		}
		ext := strings.ToLower(filepath.Ext(fv.Path))
		rs := rulesByExt[ext]
		if len(rs) == 0 {
			return
		}
		f, err := os.Open(fv.AbsPath)
		if err != nil {
			return
		}
		out.Files++
		scn := bufio.NewScanner(f)
		scn.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
		for scn.Scan() {
			line := scn.Text()
			for _, r := range rs {
				if m := r.re.FindStringSubmatch(line); m != nil {
					mod := ""
					if r.modGroup >= 0 && r.modGroup < len(m) {
						mod = m[r.modGroup]
					}
					edge := t.X1Edge{From: filepath.ToSlash(fv.Path), Module: mod}
					if strings.HasPrefix(mod, ".") || strings.HasPrefix(mod, "/") {
						baseDir := filepath.Dir(fv.Path)
						cand := filepath.Clean(filepath.Join(baseDir, mod))
						// Prefer resolution via provided index set; if empty, fallback to filesystem checks
						if _, ok := filesSet[filepath.ToSlash(cand)]; ok {
							edge.To = filepath.ToSlash(cand)
							edge.Reason = "resolved exact"
						} else {
							resolved := false
							for _, e := range []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"} {
								if _, ok := filesSet[filepath.ToSlash(cand+e)]; ok {
									edge.To = filepath.ToSlash(cand + e)
									edge.Reason = "resolved with ext " + e
									resolved = true
									break
								}
								ix := filepath.Join(cand, "index"+e)
								if _, ok := filesSet[filepath.ToSlash(ix)]; ok {
									edge.To = filepath.ToSlash(ix)
									edge.Reason = "resolved index with ext " + e
									resolved = true
									break
								}
							}
							if !resolved && len(filesSet) == 0 {
								// fallback: check filesystem
								if _, err := os.Stat(filepath.Join(in.Repo, filepath.FromSlash(cand))); err == nil {
									edge.To = filepath.ToSlash(cand)
									edge.Reason = "resolved exact (fs)"
								} else {
									for _, e := range []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"} {
										if _, err := os.Stat(filepath.Join(in.Repo, filepath.FromSlash(cand+e))); err == nil {
											edge.To = filepath.ToSlash(cand + e)
											edge.Reason = "resolved with ext (fs) " + e
											break
										}
										ix := filepath.Join(cand, "index"+e)
										if _, err := os.Stat(filepath.Join(in.Repo, filepath.FromSlash(ix))); err == nil {
											edge.To = filepath.ToSlash(ix)
											edge.Reason = "resolved index with ext (fs) " + e
											break
										}
									}
								}
							}
						}
					} else {
						edge.Reason = "external module"
					}
					out.Edges = append(out.Edges, edge)
					out.Matches++
				}
			}
		}
		_ = f.Close()
	})
	return out, nil
}
