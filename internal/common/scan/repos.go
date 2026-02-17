package scan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	reposDirMu sync.RWMutex
	reposDir   = defaultReposDir()
)

func defaultReposDir() string {
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, "repos")
	}
	return filepath.Join(".", "repos")
}

// SetReposDir overrides the base directory that contains all repositories.
// Tests can use this to isolate filesystem operations.
func SetReposDir(dir string) {
	if dir == "" {
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = filepath.Clean(dir)
	}
	reposDirMu.Lock()
	reposDir = abs
	reposDirMu.Unlock()
}

// ReposDir returns the current base directory for repositories.
func ReposDir() string {
	reposDirMu.RLock()
	defer reposDirMu.RUnlock()
	return reposDir
}

// ResolveRepo converts a single-segment repo name (e.g., "CoinApi") into an
// absolute path under ReposDir. It rejects empty names, names containing path
// separators, or directories outside of ReposDir.
func ResolveRepo(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("scan: repo name is required")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("scan: repo name %q must not contain path separators or ..", name)
	}
	base := ReposDir()
	if base == "" {
		return "", errors.New("scan: repos dir is not configured")
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	path := filepath.Join(baseAbs, name)
	fi, err := safeFS().SafeStat(path)
	if err != nil {
		return "", fmt.Errorf("scan: repo %q not found under %s: %w", name, baseAbs, err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("scan: repo %q is not a directory", name)
	}
	return path, nil
}

// ResolveRoot accepts either a repo name (single segment) or a filesystem path
// and returns the absolute root directory to scan. Paths must live under
// ReposDir; repo names are resolved relative to it.
func ResolveRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("scan: root is empty")
	}
	if isRepoName(root) {
		return ResolveRepo(root)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return ensureRootAllowed(abs)
}

func isRepoName(s string) bool {
	if s == "" {
		return false
	}
	if s == "." || s == ".." {
		return false
	}
	return !strings.ContainsAny(s, `/\`)
}

func ensureRootAllowed(root string) (string, error) {
	if root == "" {
		return "", errors.New("scan: root is empty")
	}
	base := ReposDir()
	if base == "" {
		return "", errors.New("scan: repos dir is not configured")
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if !hasPathPrefix(rootAbs, baseAbs) {
		return "", fmt.Errorf("scan: root %s is outside allowed repos dir %s", rootAbs, baseAbs)
	}
	return rootAbs, nil
}

func hasPathPrefix(path, base string) bool {
	path = filepath.Clean(path)
	base = filepath.Clean(base)
	if len(base) == 0 {
		return true
	}
	if path == base {
		return true
	}
	sep := string(os.PathSeparator)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
		base = strings.ToLower(base)
	}
	if !strings.HasSuffix(base, sep) {
		base += sep
	}
	if !strings.HasSuffix(path, sep) {
		path += sep
	}
	return strings.HasPrefix(path, base)
}
