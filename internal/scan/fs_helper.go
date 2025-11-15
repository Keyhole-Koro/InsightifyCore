package scan

import (
	"log"
	"sync"

	"insightify/internal/safeio"
)

var (
	fsOverrideMu sync.RWMutex
	fsOverride   *safeio.SafeFS
)

// SetSafeFS overrides the filesystem used by scan (primarily for tests).
func SetSafeFS(fs *safeio.SafeFS) {
	fsOverrideMu.Lock()
	fsOverride = fs
	fsOverrideMu.Unlock()
}

// CurrentSafeFS returns the currently configured scan filesystem (override or default).
func CurrentSafeFS() *safeio.SafeFS {
	fsOverrideMu.RLock()
	fs := fsOverride
	fsOverrideMu.RUnlock()
	if fs != nil {
		return fs
	}
	return safeio.Default()
}

func safeFS() *safeio.SafeFS {
	if fs := CurrentSafeFS(); fs != nil {
		return fs
	}
	log.Fatal("scan: safe filesystem is not configured")
	return nil
}
