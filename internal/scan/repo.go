package scan

import (
    "io/fs"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "time"
)

type FileMeta struct {
    Path    string
    Size    int64
    LOC     int
    ModTime time.Time
    Ext     string
}

type RepoTree struct {
    Root  string
    Files []FileMeta
}

var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true}

func Scan(root string) (RepoTree, error) {
    var files []FileMeta
    err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
        if err != nil { return err }
        if d.IsDir() {
            if skipDirs[d.Name()] {
                return filepath.SkipDir
            }
            return nil
        }
        info, _ := d.Info()
        rel := strings.TrimPrefix(p, root+string(os.PathSeparator))
        if isBinary(rel) { return nil }
        files = append(files, FileMeta{Path: rel, Size: info.Size(), LOC: quickLOC(p), ModTime: info.ModTime(), Ext: strings.ToLower(filepath.Ext(rel))})
        return nil
    })
    return RepoTree{Root: root, Files: files}, err
}

func quickLOC(p string) int { b, err := os.ReadFile(p); if err != nil { return 0 }; return strings.Count(string(b), "\n") + 1 }

// Headings from markdown/rst files (H1-H3)
func ExtractDocHeadings(root string, t RepoTree) map[string][]string {
    heads := map[string][]string{}
    re := regexp.MustCompile(`(?m)^(#{1,3})\s+(.+)$`)
    for _, f := range t.Files {
        if strings.HasSuffix(f.Path, ".md") || strings.HasSuffix(f.Path, ".rst") {
            b, err := os.ReadFile(filepath.Join(root, f.Path)); if err != nil { continue }
            ms := re.FindAllStringSubmatch(string(b), -1)
            for _, m := range ms { heads[f.Path] = append(heads[f.Path], m[2]) }
        }
    }
    return heads
}

// Read key manifests lightly (first ~4KB)
func CollectManifests(root string, t RepoTree) map[string]string {
    keys := []string{"package.json","go.mod","requirements.txt","pom.xml","build.gradle","Cargo.toml","Dockerfile","docker-compose.yml"}
    out := map[string]string{}
    for _, f := range t.Files {
        for _, k := range keys {
            if filepath.Base(f.Path) == k {
                b, err := os.ReadFile(filepath.Join(root, f.Path)); if err == nil {
                    if len(b) > 4000 { b = b[:4000] }
                    out[f.Path] = string(b)
                }
            }
        }
    }
    return out
}

func (r RepoTree) FindByPath(path string) (FileMeta, bool) {
    for _, f := range r.Files { if f.Path == path { return f, true } }
    return FileMeta{}, false
}

func isBinary(path string) bool {
    switch strings.ToLower(filepath.Ext(path)) {
    // images
    case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".bmp", ".tiff", ".svg":
        return true
    // video
    case ".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi":
        return true
    // audio
    case ".mp3", ".wav", ".ogg", ".flac", ".m4a":
        return true
    // archives / others
    case ".pdf", ".zip", ".jar", ".gz", ".tgz", ".bz2", ".7z", ".exe", ".dll", ".dylib", ".so", ".woff", ".woff2":
        return true
    }
    return false
}