package scan

import (
    "io/fs"
    "os"
    "path/filepath"
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
        ext := strings.ToLower(filepath.Ext(rel))
        files = append(files, FileMeta{Path: rel, Size: info.Size(), LOC: quickLOC(p), ModTime: info.ModTime(), Ext: ext})
        return nil
    })
    return RepoTree{Root: root, Files: files}, err
}

func quickLOC(p string) int {
    b, err := os.ReadFile(p); if err != nil { return 0 }
    return strings.Count(string(b), "\n") + 1
}

func (r RepoTree) FindByPath(path string) (FileMeta, bool) {
    for _, f := range r.Files {
        if f.Path == path {
            return f, true
        }
    }
    return FileMeta{}, false
}
