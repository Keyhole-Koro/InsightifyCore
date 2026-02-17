package runner

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"insightify/internal/common/safeio"
)

// JSONFingerprint computes a stable hash of any JSON-serializable value.
func JSONFingerprint(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:])[:16]
}

// FileExists checks if a file exists and is not a directory.
func FileExists(fs *safeio.SafeFS, path string) bool {
	fs = ensureFS(fs)
	fi, err := fs.SafeStat(path)
	return err == nil && !fi.IsDir()
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func ensureFS(fs *safeio.SafeFS) *safeio.SafeFS {
	if fs != nil {
		return fs
	}
	if dfs := safeio.Default(); dfs != nil {
		return dfs
	}
	log.Fatal("safe filesystem is not configured")
	return nil
}
