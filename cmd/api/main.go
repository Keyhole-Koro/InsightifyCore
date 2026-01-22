package main

import (
	"flag"
	"log"
	"net/http"
)

// Entry point for the Connect/HTTP API server.
func main() {
	addr := flag.String("addr", ":8080", "listen address")
	reposRoot := flag.String("repos_root", "", "base directory for checked-out repos (optional, currently informational)")
	repo := flag.String("repo", "", "default repo name to serve (optional, currently informational)")
	flag.Parse()

	srv := newAPIServer()
	mux := buildMux(srv)

	if *reposRoot != "" || *repo != "" {
		log.Printf("repos_root=%q repo=%q (flags accepted for compatibility; not yet used by API server)", *reposRoot, *repo)
	}
	log.Printf("connect API listening on %s", *addr)
	if err := http.ListenAndServe(*addr, withCORS(mux)); err != nil {
		log.Fatal(err)
	}
}
