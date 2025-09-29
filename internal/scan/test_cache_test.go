package scan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeTree creates a directory tree with many files to make the first scan non-trivial.
func makeTree(t *testing.T, root string, dirs, filesPerDir int) int {
	t.Helper()
	total := 0
	for d := 0; d < dirs; d++ {
		dir := filepath.Join(root, "dir_"+itoa(d))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		for f := 0; f < filesPerDir; f++ {
			p := filepath.Join(dir, "file_"+itoa(f)+".txt")
			if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			total++
		}
	}
	return total
}

// minimal itoa to avoid fmt in hot loop
func itoa(x int) string {
	if x == 0 {
		return "0"
	}
	neg := x < 0
	if neg {
		x = -x
	}
	var buf [20]byte
	i := len(buf)
	for x > 0 {
		i--
		buf[i] = byte('0' + x%10)
		x /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestWholeCacheImprovesScanTime(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Create 200 directories x 10 files = 2000 files
	makeTree(t, dir, 200, 10)

	ClearCache()
	// First scan (populate cache)
	start1 := time.Now()
	var n1 int
	if err := ScanWithOptions(dir, Options{}, func(f FileVisit) { n1++ }); err != nil {
		t.Fatalf("scan1: %v", err)
	}
	d1 := time.Since(start1)

	// Second scan (should hit whole-cache path)
	start2 := time.Now()
	var n2 int
	if err := ScanWithOptions(dir, Options{}, func(f FileVisit) { n2++ }); err != nil {
		t.Fatalf("scan2: %v", err)
	}
	d2 := time.Since(start2)

	if n1 != n2 {
		t.Fatalf("expected same entry count, got n1=%d n2=%d", n1, n2)
	}

	// Only assert a speedup when the first run took a meaningful amount of time.
	// Expect at least a 2x improvement.
	if d1 > 15*time.Millisecond {
		if !(d2*2 < d1) {
			t.Fatalf("expected cached scan faster (d1=%v, d2=%v)", d1, d2)
		}
	}
}

func TestBypassCacheSeesNewFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_ = makeTree(t, dir, 20, 5) // 100 files

	ClearCache()
	// Populate cache
	var n1 int
	if err := ScanWithOptions(dir, Options{}, func(f FileVisit) { n1++ }); err != nil {
		t.Fatalf("seed scan: %v", err)
	}

	// Add a brand new file after cache is warm
	newPath := filepath.Join(dir, "brand_new.txt")
	if err := os.WriteFile(newPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write new: %v", err)
	}

	// Cached path should not observe the new file
	seenCached := false
	if err := ScanWithOptions(dir, Options{}, func(f FileVisit) {
		if filepath.ToSlash(f.Path) == "brand_new.txt" {
			seenCached = true
		}
	}); err != nil {
		t.Fatalf("cached scan: %v", err)
	}
	if seenCached {
		t.Fatalf("expected cached scan to miss new file")
	}

	// BypassCache should see the new file
	seenBypass := false
	if err := ScanWithOptions(dir, Options{BypassCache: true}, func(f FileVisit) {
		if filepath.ToSlash(f.Path) == "brand_new.txt" {
			seenBypass = true
		}
	}); err != nil {
		t.Fatalf("bypass scan: %v", err)
	}
	if !seenBypass {
		t.Fatalf("expected bypass scan to include new file")
	}
}
