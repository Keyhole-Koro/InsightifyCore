package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/anypb"
	pipelinev1 "insightify/gen/go/pipeline/v1"
	"insightify/gen/go/pipeline/v1/pipelinev1connect"
	"insightify/internal/llm"
	"insightify/internal/mcp"
	"insightify/internal/runner"
	"insightify/internal/safeio"
)

// RunPipeline executes the requested phases and streams GatewayEvents (ProgressEvent + PhaseResult).
func (s *apiServer) RunPipeline(ctx context.Context, req *connect.Request[pipelinev1.RunPipelineRequest], stream *connect.ServerStream[pipelinev1.GatewayEvent]) error {
	repoPath := strings.TrimSpace(req.Msg.GetRepoPath())
	if repoPath == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("repo_path is required"))
	}
	outDir := strings.TrimSpace(req.Msg.GetOutDir())
	if outDir == "" {
		outDir = filepath.Join(repoPath, ".insightify")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("mkdir out_dir: %w", err))
	}

	repoFS, err := safeio.NewSafeFS(repoPath)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repo fs: %w", err))
	}
	artifactFS, err := safeio.NewSafeFS(outDir)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("artifact fs: %w", err))
	}
	safeio.SetDefault(repoFS)

	llmCli := llm.Wrap(llm.NewFakeClient(4096), llm.WithHooks())
	defer llmCli.Close()

	env := &runner.Env{
		Repo:         filepath.Base(repoPath),
		RepoRoot:     repoPath,
		OutDir:       outDir,
		MaxNext:      8,
		RepoFS:       repoFS,
		ArtifactFS:   artifactFS,
		ModelSalt:    "gateway|fake",
		ForceFrom:    "",
		LLM:          llmCli,
		StripImgMD:   regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`),
		StripImgHTML: regexp.MustCompile(`(?is)<img[^>]*>`),
	}
	env.MCPHost = mcp.Host{RepoRoot: repoPath, RepoFS: repoFS, ArtifactFS: artifactFS}
	env.MCP = mcp.NewRegistry()
	mcp.RegisterDefaultTools(env.MCP, env.MCPHost)

	mainline := runner.BuildRegistryMainline(env)
	codebase := runner.BuildRegistryCodebase(env)
	external := runner.BuildRegistryExternal(env)
	env.Resolver = runner.MergeRegistries(mainline, codebase, external)

	phaseKeys := req.Msg.GetPhaseKeys()
	if len(phaseKeys) == 0 {
		phaseKeys = []string{"m0", "m1"}
	}

	for _, key := range phaseKeys {
		spec, ok := env.Resolver.Get(key)
		if !ok {
			return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown phase %s", key))
		}
		sendProgress(stream, fmt.Sprintf("starting %s", key))
		result, err := runner.ExecutePhaseWithResult(ctx, spec, env)
		if err != nil {
			sendProgress(stream, fmt.Sprintf("failed %s: %v", key, err))
			return err
		}
		view := result.ClientView
		if view != nil {
			switch v := view.(type) {
			case *pipelinev1.PhaseResult:
				if v.GetPhase() == "" {
					v.Phase = key
				}
				if err := stream.Send(&pipelinev1.GatewayEvent{
					Event: &pipelinev1.GatewayEvent_Result{Result: v},
				}); err != nil {
					return err
				}
			default:
				anyView, _ := anypb.New(view)
				res := &pipelinev1.PhaseResult{
					Phase: key,
					Nodes: nil,
				}
				if err := stream.Send(&pipelinev1.GatewayEvent{
					Event: &pipelinev1.GatewayEvent_Result{Result: res},
				}); err != nil {
					return err
				}
				_ = anyView // unused placeholder; future types can be handled here
			}
		}
		sendProgress(stream, fmt.Sprintf("completed %s", key))
	}
	return nil
}

func sendProgress(stream *connect.ServerStream[pipelinev1.GatewayEvent], msg string) {
	_ = stream.Send(&pipelinev1.GatewayEvent{
		Event: &pipelinev1.GatewayEvent_Progress{
			Progress: &pipelinev1.ProgressEvent{
				Payload: &pipelinev1.ProgressEvent_Status{
					Status: &pipelinev1.StatusPayload{Message: msg},
				},
			},
		},
	})
}

// Ensure interface conformance
var _ pipelinev1connect.GatewayServiceHandler = (*apiServer)(nil)
