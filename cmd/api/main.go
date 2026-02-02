package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"insightify/internal/safeio"
	"insightify/internal/scan"
)

func main() {
	port := flag.String("port", ":8080", "server port")
	flag.Parse()

	_ = godotenv.Load()

	server := newAPIServer()
	mux := buildMux(server)

	// Simple CORS middleware
	handler := http.Handler(mux)
	handler = func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, Connect-Protocol-Version")
			if r.Method == "OPTIONS" {
				return
			}
			h.ServeHTTP(w, r)
		})
	}(handler)

	log.Printf("Starting API server on %s", *port)
	log.Fatal(http.ListenAndServe(*port, h2c.NewHandler(handler, &http2.Server{})))
}

// Helpers duplicated from archflow/main.go for standalone compilation
func resolveRepoPaths(repo string) (string, string, *safeio.SafeFS, error) {
	repoName := strings.TrimSpace(repo)
	if repoName == "" {
		return "", "", nil, fmt.Errorf("--repo is required")
	}
	reposRoot := strings.TrimSpace(os.Getenv("REPOS_ROOT"))
	if reposRoot == "" {
		return "", "", nil, fmt.Errorf("REPOS_ROOT must be set")
	}
	if abs, err := filepath.Abs(reposRoot); err == nil {
		reposRoot = abs
	}
	scan.SetReposDir(reposRoot)
	repoPath, err := scan.ResolveRepo(repoName)
	if err != nil {
		return "", "", nil, err
	}
	sfs, err := safeio.NewSafeFS(repoPath)
	if err != nil {
		return "", "", nil, err
	}
	return repoName, repoPath, sfs, nil
}
