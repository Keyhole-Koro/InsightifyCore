package worker

import (
	"context"
	"fmt"
	"log"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/runner"
	"io/fs"
	"os"
	"path/filepath"
)

type runState struct {
	runID     string
	projectID string
	workerID  string
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

	runID := s.newRunID()
	runCtx, cancel := context.WithCancel(context.Background())
	st := &runState{
		runID:     runID,
		projectID: projectID,
		workerID:  workerID,
	}

	s.runMu.Lock()
	s.runs[runID] = st
	s.runMu.Unlock()

	go func() {
		defer cancel()
		s.executeRun(runCtx, runID, projectID, workerID, req.GetParams())
	}()

	return &insightifyv1.StartRunResponse{RunId: runID}, nil
}

func (s *Service) newRunID() string {
	n := s.runCounter.Add(1)
	return fmt.Sprintf("run-%d", n)
}

func (s *Service) executeRun(ctx context.Context, runID, projectID, workerID string, params map[string]string) {
	runEnv, err := s.project.EnsureRunContext(projectID)
	if err != nil {
		log.Printf("run %s ensure context failed: %v", runID, err)
		if s.interaction != nil {
			_ = s.interaction.PublishOutput(context.Background(), runID, "", fmt.Sprintf("run setup failed: %v", err))
		}
		return
	}
	if runEnv == nil || runEnv.Runtime() == nil || runEnv.Runtime().GetResolver() == nil {
		log.Printf("run %s has no resolver", runID)
		if s.interaction != nil {
			_ = s.interaction.PublishOutput(context.Background(), runID, "", "run environment resolver is not available")
		}
		return
	}

	execCtx := runner.WithRunID(ctx, runID)
	if s.interaction != nil {
		execCtx = runner.WithInteractionWaiter(execCtx, s.interaction)
	}

	out, err := runner.ExecuteWorker(execCtx, runEnv.Runtime(), workerID, params)
	if err != nil {
		log.Printf("run %s execute worker %s failed: %v", runID, workerID, err)
		if s.interaction != nil {
			_ = s.interaction.PublishOutput(context.Background(), runID, "", fmt.Sprintf("worker failed: %v", err))
		}
		return
	}

	clientView := asClientView(out.ClientView)
	if s.ui != nil {
		node := s.ui.UpsertFromClientView(runID, workerID, clientView)
		if node != nil && s.workspaces != nil {
			if err := s.workspaces.AssignRunToCurrentTab(projectID, runID); err != nil {
				log.Printf("failed to assign run %s to current tab for project %s: %v", runID, projectID, err)
			}
		}
	}

	// Persist artifacts
	if s.artifact != nil {
		go func() {
			if err := s.syncArtifacts(context.Background(), runID, runEnv.GetOutDir()); err != nil {
				log.Printf("failed to sync artifacts for run %s: %v", runID, err)
			}
		}()
	}
}

func (s *Service) syncArtifacts(ctx context.Context, runID, outDir string) error {
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
		return s.artifact.Put(ctx, runID, rel, content)
	})
}
