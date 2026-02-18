package artifactfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileStore provides local filesystem-backed artifact access rooted at a base directory.
type FileStore struct {
	root string
}

func NewFileStore(root string) *FileStore {
	return &FileStore{root: strings.TrimSpace(root)}
}

func (s *FileStore) Read(_ context.Context, name string) ([]byte, error) {
	path, err := s.pathFor(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (s *FileStore) Write(_ context.Context, name string, content []byte) error {
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func (s *FileStore) Remove(_ context.Context, name string) error {
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *FileStore) List(_ context.Context) ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("artifact store is not configured")
	}
	root := strings.TrimSpace(s.root)
	if root == "" {
		return nil, fmt.Errorf("artifact root is required")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}

func (s *FileStore) pathFor(name string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("artifact store is not configured")
	}
	root := strings.TrimSpace(s.root)
	if root == "" {
		return "", fmt.Errorf("artifact root is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("artifact name is required")
	}
	if strings.Contains(name, "..") || filepath.IsAbs(name) {
		return "", fmt.Errorf("invalid artifact name: %s", name)
	}
	return filepath.Join(root, name), nil
}
