package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/pipeline/plan"
	"insightify/internal/runner"
)

// DeclareGraphSpec registers a spec so PlanPipeline can reuse it by id.
func (s *apiServer) DeclareGraphSpec(ctx context.Context, req *connect.Request[insightifyv1.DeclareGraphSpecRequest]) (*connect.Response[insightifyv1.DeclareGraphSpecResponse], error) {
	spec := req.Msg.GetSpec()
	if spec == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("spec is required"))
	}
	if _, err := normalizeSpec(spec); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if !req.Msg.GetDryRun() {
		s.saveSpec(spec)
	}
	return connect.NewResponse(&insightifyv1.DeclareGraphSpecResponse{Spec: spec}), nil
}

// PlanPipeline builds a deterministic DAG based on the registry descriptors and requested capabilities.
func (s *apiServer) PlanPipeline(ctx context.Context, req *connect.Request[insightifyv1.PlanPipelineRequest]) (*connect.Response[insightifyv1.PlanPipelineResponse], error) {
	_ = ctx
	spec := req.Msg.GetSpec()
	if spec == nil && req.Msg.GetSpecId() != "" {
		if cached, ok := s.loadSpec(req.Msg.GetSpecId()); ok {
			spec = cached
		}
	}
	if spec == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("spec is required (inline or via spec_id)"))
	}
	spec = cloneSpec(spec)
	planSpec, err := normalizeSpec(spec)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	mergePipelineCapabilities(spec, &planSpec, req.Msg.GetPipelineKeys())

	descs := runner.BuildPhaseDescriptors()
	built, warnings := plan.BuildPlanFromSpec(planSpec, descs)
	if len(warnings) > 0 {
		built.Warnings = append(built.Warnings, warnings...)
	}
	resp := &insightifyv1.PlanPipelineResponse{
		Plan:         built,
		PhaseCatalog: toProtoPhaseDescriptors(descs),
		Pipelines:    buildPipelineOverviews(descs, req.Msg.GetPipelineKeys()),
	}
	return connect.NewResponse(resp), nil
}

// StartRun is not wired to the runner yet.
func (s *apiServer) StartRun(ctx context.Context, _ *connect.Request[insightifyv1.StartRunRequest]) (*connect.Response[insightifyv1.StartRunResponse], error) {
	_ = ctx
	now := timestamppb.Now()
	run := &insightifyv1.Run{
		Id:        fmt.Sprintf("run-%d", time.Now().UnixNano()),
		Status:    insightifyv1.RunStatus_RUN_STATUS_RUNNING,
		StartedAt: now,
		Outputs: &insightifyv1.RunOutputs{
			Public:   nil,
			Internal: nil,
		},
	}
	s.saveRun(run)
	return connect.NewResponse(&insightifyv1.StartRunResponse{Run: sanitizeRun(run)}), nil
}

// GetRun returns a sanitized run (public outputs only).
func (s *apiServer) GetRun(ctx context.Context, req *connect.Request[insightifyv1.GetRunRequest]) (*connect.Response[insightifyv1.Run], error) {
	_ = ctx
	if req.Msg == nil || strings.TrimSpace(req.Msg.GetRunId()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("run_id is required"))
	}
	if run, ok := s.loadRun(req.Msg.GetRunId()); ok {
		return connect.NewResponse(sanitizeRun(run)), nil
	}
	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("run %s not found", req.Msg.GetRunId()))
}

// WatchRun is a placeholder until execution is integrated.
func (s *apiServer) WatchRun(ctx context.Context, _ *connect.Request[insightifyv1.WatchRunRequest], _ *connect.ServerStream[insightifyv1.RunEvent]) error {
	_ = ctx
	return connect.NewError(connect.CodeUnimplemented, errors.New("WatchRun is not implemented yet"))
}

// normalizeSpec enforces prompt + repo URL, fills defaults, and projects to the internal planning spec.
func normalizeSpec(spec *insightifyv1.GraphSpec) (plan.Spec, error) {
	if spec == nil {
		return plan.Spec{}, errors.New("spec is nil")
	}
	spec.RawSpecText = strings.TrimSpace(spec.GetRawSpecText())
	spec.RepoUrl = strings.TrimSpace(spec.GetRepoUrl())
	spec.RepoPath = strings.TrimSpace(spec.GetRepoPath())

	if spec.GetRawSpecText() == "" {
		return plan.Spec{}, errors.New("raw_spec_text (prompt) is required")
	}
	if spec.GetRepoUrl() == "" {
		return plan.Spec{}, errors.New("repo_url is required")
	}
	if spec.GetRepoPath() == "" {
		spec.RepoPath = deriveRepoPathFromURL(spec.GetRepoUrl())
	}
	if spec.GetId() == "" {
		spec.Id = fmt.Sprintf("spec-%d", time.Now().UnixNano())
	}
	caps := dedupeNormalized(spec.GetCapabilities())
	spec.Capabilities = caps
	return plan.Spec{
		ID:           spec.GetId(),
		RepoURL:      spec.GetRepoUrl(),
		RepoPath:     spec.GetRepoPath(),
		RawSpecText:  spec.GetRawSpecText(),
		Capabilities: caps,
	}, nil
}

func toProtoPhaseDescriptors(descs []plan.PhaseDescriptor) []*insightifyv1.PhaseDescriptor {
	out := make([]*insightifyv1.PhaseDescriptor, 0, len(descs))
	for _, d := range descs {
		out = append(out, phaseDescriptorToProto(d))
	}
	return out
}

func phaseDescriptorToProto(d plan.PhaseDescriptor) *insightifyv1.PhaseDescriptor {
	return &insightifyv1.PhaseDescriptor{
		Key:        d.Key,
		Summary:    d.Summary,
		Consumes:   d.Consumes,
		Produces:   d.Produces,
		Requires:   d.Requires,
		Downstream: d.Downstream,
		UsesLlm:    d.UsesLLM,
		Tags:       d.Tags,
		Metadata:   d.Metadata,
	}
}

func cloneSpec(spec *insightifyv1.GraphSpec) *insightifyv1.GraphSpec {
	if spec == nil {
		return nil
	}
	cloned, ok := proto.Clone(spec).(*insightifyv1.GraphSpec)
	if !ok {
		return spec
	}
	return cloned
}

func mergePipelineCapabilities(spec *insightifyv1.GraphSpec, planSpec *plan.Spec, pipelineKeys []string) {
	if len(pipelineKeys) == 0 {
		return
	}
	seen := map[string]struct{}{}
	var merged []string
	appendCap := func(val string) {
		n := norm(val)
		if n == "" {
			return
		}
		if _, ok := seen[n]; ok {
			return
		}
		seen[n] = struct{}{}
		merged = append(merged, n)
	}
	for _, c := range spec.GetCapabilities() {
		appendCap(c)
	}
	for _, p := range pipelineKeys {
		appendCap(p)
	}
	if spec != nil {
		spec.Capabilities = merged
	}
	if planSpec != nil {
		planSpec.Capabilities = merged
	}
}

func buildPipelineOverviews(descs []plan.PhaseDescriptor, requested []string) []*insightifyv1.PipelineOverview {
	requestedSet := map[string]struct{}{}
	for _, k := range requested {
		if nk := norm(k); nk != "" {
			requestedSet[nk] = struct{}{}
		}
	}

	phaseToPipeline := map[string]string{}
	for _, d := range descs {
		if tag := pipelineTag(d.Tags); tag != "" {
			phaseToPipeline[norm(d.Key)] = tag
		}
	}

	pipelinePhases := map[string][]plan.PhaseDescriptor{}
	for _, d := range descs {
		tag := pipelineTag(d.Tags)
		if tag == "" {
			continue
		}
		if len(requestedSet) > 0 {
			if _, ok := requestedSet[tag]; !ok {
				continue
			}
		}
		pipelinePhases[tag] = append(pipelinePhases[tag], d)
	}
	for k := range requestedSet {
		if _, ok := pipelinePhases[k]; !ok {
			pipelinePhases[k] = nil
		}
	}

	var pipelineKeys []string
	for k := range pipelinePhases {
		pipelineKeys = append(pipelineKeys, k)
	}
	sort.Strings(pipelineKeys)

	var out []*insightifyv1.PipelineOverview
	for _, key := range pipelineKeys {
		phases := pipelinePhases[key]
		sort.Slice(phases, func(i, j int) bool { return phases[i].Key < phases[j].Key })

		deps := map[string]struct{}{}
		for _, p := range phases {
			for _, req := range p.Requires {
				if depPipeline, ok := phaseToPipeline[norm(req)]; ok && depPipeline != key {
					deps[depPipeline] = struct{}{}
				}
			}
		}
		var depList []string
		for dep := range deps {
			depList = append(depList, dep)
		}
		sort.Strings(depList)

		overview := &insightifyv1.PipelineOverview{
			Key:       key,
			DependsOn: depList,
		}
		for _, p := range phases {
			overview.Phases = append(overview.Phases, phaseDescriptorToProto(p))
		}
		out = append(out, overview)
	}
	return out
}

func pipelineTag(tags []string) string {
	for _, t := range tags {
		switch norm(t) {
		case "mainline", "codebase", "external":
			return norm(t)
		}
	}
	return ""
}

func norm(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func dedupeNormalized(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, v := range values {
		n := norm(v)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

func deriveRepoPathFromURL(repoURL string) string {
	if parsed, err := url.Parse(repoURL); err == nil {
		base := path.Base(parsed.Path)
		base = strings.TrimSuffix(base, ".git")
		return base
	}
	base := path.Base(repoURL)
	return strings.TrimSuffix(base, ".git")
}

func (s *apiServer) saveSpec(spec *insightifyv1.GraphSpec) {
	if spec == nil {
		return
	}
	s.specMu.Lock()
	defer s.specMu.Unlock()
	s.specs[norm(spec.GetId())] = spec
}

func (s *apiServer) loadSpec(id string) (*insightifyv1.GraphSpec, bool) {
	s.specMu.RLock()
	defer s.specMu.RUnlock()
	spec, ok := s.specs[norm(id)]
	return spec, ok
}

func (s *apiServer) saveRun(run *insightifyv1.Run) {
	if run == nil {
		return
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.runs[run.GetId()] = run
}

func (s *apiServer) loadRun(id string) (*insightifyv1.Run, bool) {
	s.runMu.RLock()
	defer s.runMu.RUnlock()
	run, ok := s.runs[norm(id)]
	return run, ok
}

// sanitizeRun drops internal outputs and retains only public artifacts for API responses.
func sanitizeRun(run *insightifyv1.Run) *insightifyv1.Run {
	if run == nil {
		return nil
	}
	cloned := proto.Clone(run).(*insightifyv1.Run)
	if cloned.Outputs != nil {
		pub := filterPublicArtifacts(cloned.Outputs.GetPublic())
		cloned.Outputs = &insightifyv1.RunOutputs{
			Public:   pub,
			Internal: nil,
		}
	}
	return cloned
}

func filterPublicArtifacts(refs []*insightifyv1.ArtifactRef) []*insightifyv1.ArtifactRef {
	var out []*insightifyv1.ArtifactRef
	for _, r := range refs {
		if r == nil {
			continue
		}
		switch r.GetVisibility() {
		case insightifyv1.ArtifactVisibility_ARTIFACT_VISIBILITY_PUBLIC, insightifyv1.ArtifactVisibility_ARTIFACT_VISIBILITY_UNSPECIFIED:
			out = append(out, proto.Clone(r).(*insightifyv1.ArtifactRef))
		default:
			continue
		}
	}
	return out
}
