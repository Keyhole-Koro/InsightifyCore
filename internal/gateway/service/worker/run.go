package worker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	logctx "insightify/internal/common/logctx"
	traceutil "insightify/internal/common/trace"
	projectrepo "insightify/internal/gateway/repository/project"
	"insightify/internal/runner"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// WorkerRuntime tracks run-scoped execution state.
type WorkerRuntime struct {
	RunID     string
	ProjectID string
	WorkerID  string
	StartedAt time.Time
}

func (s *Service) StartRun(ctx context.Context, req *insightifyv1.StartRunRequest) (*insightifyv1.StartRunResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	projectID := strings.TrimSpace(req.GetProjectId())
	workerID := strings.TrimSpace(req.GetWorkerId())
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if workerID == "" {
		return nil, fmt.Errorf("worker_id is required")
	}

	runID := s.newRunID(projectID)
	reqTraceID := traceutil.FromContext(ctx)
	runBaseCtx := traceutil.WithContext(context.Background(), reqTraceID)
	runCtx, cancel := context.WithCancel(runBaseCtx)
	st := &WorkerRuntime{
		RunID:     runID,
		ProjectID: projectID,
		WorkerID:  workerID,
		StartedAt: time.Now(),
	}
	logctx.Info(runCtx, "worker run started", "run_id", runID, "project_id", projectID, "worker_id", workerID)

	s.runMu.Lock()
	s.runs[runID] = st
	s.runMu.Unlock()

	if s.workspaces != nil {
		if err := s.workspaces.AssignRunToCurrentTab(projectID, runID); err != nil {
			logctx.Error(runCtx, "failed to assign run to current tab", err, "run_id", runID, "project_id", projectID)
		}
	}

	go func() {
		defer cancel()
		s.executeRun(runCtx, runID, projectID, workerID, req.GetParams())
	}()

	return &insightifyv1.StartRunResponse{RunId: runID}, nil
}

func (s *Service) newRunID(projectID string) string {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		pid = "project"
	}
	ts := time.Now().UnixMilli()
	suffix := randomHex(4)
	return fmt.Sprintf("run-%s-%d-%s", pid, ts, suffix)
}

func randomHex(size int) string {
	if size <= 0 {
		size = 4
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func (s *Service) executeRun(ctx context.Context, runID, projectID, workerID string, params map[string]string) {
	runEnv, err := s.project.EnsureRunContext(projectID)
	if err != nil {
		logctx.Error(ctx, "run ensure context failed", err, "run_id", runID, "project_id", projectID, "worker_id", workerID)
		return
	}
	if runEnv == nil || runEnv.Runtime() == nil || runEnv.Runtime().GetResolver() == nil {
		logctx.Error(ctx, "run has no resolver", nil, "run_id", runID, "project_id", projectID, "worker_id", workerID)
		return
	}

	execCtx := runner.WithRunID(ctx, runID)
	if nodeID := strings.TrimSpace(params["node_id"]); nodeID != "" {
		execCtx = runner.WithNodeID(execCtx, nodeID)
	}
	if s.interaction != nil {
		execCtx = runner.WithInteractionWaiter(execCtx, s.interaction)
	}

	out, err := runner.ExecuteWorker(execCtx, runEnv.Runtime(), workerID, params)
	if err != nil {
		logctx.Error(ctx, "execute worker failed", err, "run_id", runID, "project_id", projectID, "worker_id", workerID)
		return
	}

	clientView := asClientView(out.ClientView)
	if s.ui != nil {
		_ = s.ui.UpsertFromClientView(runID, workerID, clientView)
	}

	// Persist artifacts
	if s.artifact != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			ctx = traceutil.WithContext(ctx, traceutil.FromContext(execCtx))
			if err := s.syncArtifacts(ctx, runID, projectID, runEnv.GetOutDir()); err != nil {
				logctx.Error(ctx, "failed to sync artifacts", err, "run_id", runID, "project_id", projectID, "worker_id", workerID)
			}
		}()
	}
	logctx.Info(execCtx, "worker run completed", "run_id", runID, "project_id", projectID, "worker_id", workerID)
}

func (s *Service) syncArtifacts(ctx context.Context, runID, projectID, outDir string) error {
	return filepath.WalkDir(outDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(outDir, path)
		if err != nil {
			return nil
		}
		// Skip hidden files or internal dirs if needed, but for now persist all
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		// Normalize path to forward slashes
		rel = filepath.ToSlash(rel)
		if err := s.artifact.Put(ctx, runID, rel, content); err != nil {
			return err
		}

		if s.projectStore != nil {
			// Save metadata to project store
			_ = s.projectStore.AddArtifact(ctx, projectrepo.ProjectArtifact{
				ProjectID: projectID,
				RunID:     runID,
				Path:      rel,
			})
		}
		return nil
	})
}
