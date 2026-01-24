package runner

import (
	"context"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	mlpipe "insightify/internal/pipeline/mainline"
)

// BuildRegistryMainline defines m0/m1/m2 in one place.
// Add/modify phases here without touching main or execution logic.
func BuildRegistryMainline(env *Env) map[string]PhaseSpec {
	reg := map[string]PhaseSpec{}
	reg["m0"] = PhaseSpec{
		Key:         "m0",
		File:        "m0.json",
		Description: "Scan repo layout and ask LLM to classify main source roots, library/vendor roots, and config hotspots.",
		Consumes:    []string{"repo_structure"},
		Produces:    []string{"layout_roots"},
		UsesLLM:     true,
		Tags:        []string{"mainline", "layout"},
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			return artifact.M0In{Repo: deps.Repo()}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "m0")
			p := mlpipe.M0{LLM: env.LLM, Tools: env.MCP}
			out, err := p.Run(ctx, in.(artifact.M0In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.M0In
				Salt string
			}{in.(artifact.M0In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["m1"] = PhaseSpec{
		Key:         "m1",
		File:        "m1.json",
		Requires:    []string{"m0"},
		Description: "LLM drafts initial architecture hypothesis from file index + Markdown docs and proposes next files to open.",
		Consumes:    []string{"layout_roots", "file_index", "markdown_docs"},
		Produces:    []string{"architecture_hypothesis"},
		UsesLLM:     true,
		Tags:        []string{"mainline", "architecture"},
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var m0prev artifact.M0Out
			if err := deps.Artifact("m0", &m0prev); err != nil {
				return nil, err
			}
			ig := UniqueStrings(baseNames(m0prev.LibraryRoots...)...)
			return artifact.M1In{
				Repo:       deps.Repo(),
				IgnoreDirs: ig,
				Hints:      &artifact.M1Hints{},
				Limits:     &artifact.M1Limits{MaxNext: deps.Env().MaxNext},
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "m1")
			p := mlpipe.M1{LLM: env.LLM}
			out, err := p.Run(ctx, in.(artifact.M1In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.M1In
				Salt string
			}{in.(artifact.M1In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["m2"] = PhaseSpec{
		Key:         "m2",
		File:        "m2.json",
		Requires:    []string{"m1", "m0"}, // Explicit m0 dependency added
		Description: "LLM iterates on architecture with opened snippets/word search, emitting deltas and follow-ups.",
		Consumes:    []string{"architecture_hypothesis", "opened_files", "word_index"},
		Produces:    []string{"architecture_delta"},
		UsesLLM:     true,
		Tags:        []string{"mainline", "delta"},
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var m1 artifact.M1Out
			if err := deps.Artifact("m1", &m1); err != nil {
				return nil, err
			}
			var m0prev artifact.M0Out
			if err := deps.Artifact("m0", &m0prev); err != nil {
				return nil, err
			}

			return artifact.M2In{
				Repo:         deps.Repo(),
				RepoRoot:     deps.Root(),
				Roots:        &m0prev,
				Previous:     m1,
				LimitMaxNext: deps.Env().MaxNext,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "m2")
			p := mlpipe.M2{LLM: env.LLM}
			out, err := p.Run(ctx, in.(artifact.M2In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.M2In
				Salt string
			}{in.(artifact.M2In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	return reg
}
