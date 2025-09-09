package scan

import (
    "os"
    "path/filepath"
    "testing"
)

func TestGetSizeAndPreviewCache(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "a.txt")
    if err := os.WriteFile(p, []byte("hello world"), 0o644); err != nil { t.Fatal(err) }
    ClearFileInfoCache()

    // First call populates cache
    sz1, err := GetSize(dir, "a.txt")
    if err != nil || sz1 <= 0 { t.Fatalf("GetSize1: %v, sz=%d", err, sz1) }

    // Modify file, but without clearing cache we should still see old size
    if err := os.WriteFile(p, []byte("hello world!!!"), 0o644); err != nil { t.Fatal(err) }
    sz2, err := GetSize(dir, "a.txt")
    if err != nil { t.Fatalf("GetSize2: %v", err) }
    if sz2 != sz1 {
        t.Fatalf("expected cached size unchanged, got %d want %d", sz2, sz1)
    }

    // Preview grows with larger limits and is cached
    s5, err := GetPreview(dir, "a.txt", 5)
    if err != nil || s5 != "hello" { t.Fatalf("preview5: %q err=%v", s5, err) }
    s3, err := GetPreview(dir, "a.txt", 3)
    if err != nil || s3 != "hel" { t.Fatalf("preview3: %q err=%v", s3, err) }
    s8, err := GetPreview(dir, "a.txt", 8)
    if err != nil || s8 != "hello wo" { t.Fatalf("preview8: %q err=%v", s8, err) }

    // After clearing cache, size should reflect new content length
    ClearFileInfoCache()
    sz3, err := GetSize(dir, "a.txt")
    if err != nil || sz3 <= sz1 { t.Fatalf("GetSize3: %v sz3=%d sz1=%d", err, sz3, sz1) }
}

