package safeio

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// SafeFS provides read-only helpers that resolve paths relative to a fixed root.
type SafeFS struct {
	absRoot string // absolute root with symlinks resolved
}

var (
	defaultFSMu sync.RWMutex
	defaultFS   *SafeFS
)

func init() {
	if fsys, err := NewSafeFS("."); err == nil {
		defaultFS = fsys
	}
}

// SetDefault replaces the process-wide default SafeFS used by packages without
// explicit dependency injection (e.g., scan/wordidx). Passing nil clears it.
func SetDefault(fs *SafeFS) {
	defaultFSMu.Lock()
	defaultFS = fs
	defaultFSMu.Unlock()
}

// Default returns the process-wide SafeFS (if any).
func Default() *SafeFS {
	defaultFSMu.RLock()
	defer defaultFSMu.RUnlock()
	return defaultFS
}

// NewSafeFS locks all future operations to the given root directory.
// The root path is resolved to an absolute, symlink-free directory.
func NewSafeFS(root string) (*SafeFS, error) {
	if root == "" {
		return nil, errors.New("safeio: empty root")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("safeio: root is not a directory")
	}
	return &SafeFS{absRoot: abs}, nil
}

// Root returns the absolute root directory bound to this SafeFS.
func (s *SafeFS) Root() string {
	if s == nil {
		return ""
	}
	return s.absRoot
}

// SafeReadFile reads a file relative to the root.
func (s *SafeFS) SafeReadFile(userPath string) ([]byte, error) {
	p, err := s.resolve(userPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errors.New("safeio: path is a directory")
	}
	return os.ReadFile(p)
}

// SafeOpen opens a file relative to the root for reading.
func (s *SafeFS) SafeOpen(userPath string) (*os.File, error) {
	p, err := s.resolve(userPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errors.New("safeio: path is a directory")
	}
	return os.Open(p)
}

// SafeStat returns metadata for a file or directory under the root.
func (s *SafeFS) SafeStat(userPath string) (fs.FileInfo, error) {
	p, err := s.resolve(userPath)
	if err != nil {
		return nil, err
	}
	return os.Stat(p)
}

// SafeReadDir lists entries for a directory relative to the root.
func (s *SafeFS) SafeReadDir(userPath string) ([]fs.DirEntry, error) {
	dir, err := s.resolve(userPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("safeio: path is not a directory")
	}
	return os.ReadDir(dir)
}

// Open implements the fs.FS interface (names use "/" separators).
func (s *SafeFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	return s.SafeOpen(filepath.FromSlash(name))
}

func (s *SafeFS) resolve(userPath string) (string, error) {
	if s == nil {
		return "", errors.New("safeio: filesystem not configured")
	}
	if userPath == "" {
		return "", errors.New("safeio: empty path")
	}
	clean := filepath.Clean(userPath)
	if clean == "." {
		return s.absRoot, nil
	}

	isAbs := filepath.IsAbs(clean) || (runtime.GOOS == "windows" && filepath.VolumeName(clean) != "")
	if !isAbs {
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", errors.New("safeio: path traversal not allowed")
		}
	}

	var joined string
	if isAbs {
		joined = clean
	} else {
		joined = filepath.Join(s.absRoot, clean)
	}

	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", err
	}
	if !hasPathPrefix(resolved, s.absRoot) {
		return "", fmt.Errorf("safeio: resolved outside root (root=%s, path=%s)", s.absRoot, resolved)
	}
	return resolved, nil
}

func hasPathPrefix(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
		root = strings.ToLower(root)
	}
	if len(root) == 0 {
		return true
	}
	if path == root {
		return true
	}
	sep := string(os.PathSeparator)
	if !strings.HasSuffix(root, sep) {
		root += sep
	}
	if !strings.HasSuffix(path, sep) {
		path += sep
	}
	return strings.HasPrefix(path, root)
}
