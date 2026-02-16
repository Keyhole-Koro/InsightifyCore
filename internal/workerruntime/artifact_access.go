package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type localArtifactAccess struct {
	runtime *ExecutionRuntime
}

func newLocalArtifactAccess(rt *ExecutionRuntime) *localArtifactAccess {
	return &localArtifactAccess{runtime: rt}
}

func (a *localArtifactAccess) ReadWorker(key string) ([]byte, error) {
	if a == nil || a.runtime == nil {
		return nil, fmt.Errorf("artifact access is not configured")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("worker key is required")
	}
	artifactKey := strings.ToLower(key)
	if a.runtime.GetResolver() != nil {
		if spec, ok := a.runtime.GetResolver().Get(key); ok {
			if v := strings.TrimSpace(spec.Key); v != "" {
				artifactKey = v
			}
		}
	}
	return a.Read(artifactKey + ".json")
}

func (a *localArtifactAccess) Read(name string) ([]byte, error) {
	path, err := a.pathFor(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (a *localArtifactAccess) Write(name string, content []byte) error {
	path, err := a.pathFor(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func (a *localArtifactAccess) Remove(name string) error {
	path, err := a.pathFor(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (a *localArtifactAccess) List() ([]string, error) {
	if a == nil || a.runtime == nil {
		return nil, fmt.Errorf("artifact access is not configured")
	}
	entries, err := os.ReadDir(a.runtime.GetOutDir())
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
	return out, nil
}

func (a *localArtifactAccess) pathFor(name string) (string, error) {
	if a == nil || a.runtime == nil {
		return "", fmt.Errorf("artifact access is not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("artifact name is required")
	}
	if strings.Contains(name, "..") || filepath.IsAbs(name) {
		return "", fmt.Errorf("invalid artifact name: %s", name)
	}
	return filepath.Join(a.runtime.GetOutDir(), name), nil
}
