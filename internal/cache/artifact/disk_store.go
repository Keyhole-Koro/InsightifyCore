package artifact

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiskStore persists artifacts under a local root directory by runID/path.
type DiskStore struct {
	root string
}

func NewDiskStore(root string) *DiskStore {
	return &DiskStore{root: strings.TrimSpace(root)}
}

func (s *DiskStore) Put(_ context.Context, runID, path string, content []byte) error {
	fullPath, err := s.pathFor(runID, path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, content, 0o644)
}

func (s *DiskStore) Get(_ context.Context, runID, path string) ([]byte, error) {
	fullPath, err := s.pathFor(runID, path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(fullPath)
}

func (s *DiskStore) GetURL(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func (s *DiskStore) List(_ context.Context, runID string) ([]string, error) {
	runRoot, err := s.runRoot(runID)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, 32)
	walkErr := filepath.WalkDir(runRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(runRoot, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if walkErr != nil {
		if os.IsNotExist(walkErr) {
			return []string{}, nil
		}
		return nil, walkErr
	}
	sort.Strings(paths)
	return paths, nil
}

func (s *DiskStore) runRoot(runID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("store is nil")
	}
	root := strings.TrimSpace(s.root)
	if root == "" {
		return "", fmt.Errorf("root is required")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", fmt.Errorf("run_id is required")
	}
	if strings.Contains(runID, "..") || filepath.IsAbs(runID) {
		return "", fmt.Errorf("invalid run_id: %s", runID)
	}
	return filepath.Join(root, runID), nil
}

func (s *DiskStore) pathFor(runID, path string) (string, error) {
	runRoot, err := s.runRoot(runID)
	if err != nil {
		return "", err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.Contains(path, "..") || filepath.IsAbs(path) {
		return "", fmt.Errorf("invalid path: %s", path)
	}
	return filepath.Join(runRoot, filepath.FromSlash(path)), nil
}
