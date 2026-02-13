package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"

	"github.com/joho/godotenv"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"insightify/gen/go/insightify/v1/insightifyv1connect"
	gatewayproject "insightify/internal/gateway/project"
	gatewayrun "insightify/internal/gateway/run"
	"insightify/internal/gateway/ui"
)

func main() {
	port := flag.String("port", ":8080", "server port")
	flag.Parse()

	_ = godotenv.Load()

	uiStore := ui.NewStore()
	projectSvc := gatewayproject.New(defaultProjectStore)
	runSvc := gatewayrun.New(projectSvc.AsProjectReader(), uiStore)

	mux := http.NewServeMux()
	mux.Handle(insightifyv1connect.NewProjectServiceHandler(projectSvc))
	mux.Handle(insightifyv1connect.NewRunServiceHandler(runSvc))
	mux.HandleFunc("/debug/frontend-trace", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var in struct {
			Timestamp string         `json:"timestamp"`
			RunID     string         `json:"run_id"`
			Stage     string         `json:"stage"`
			Level     string         `json:"level"`
			Fields    map[string]any `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		runID := strings.TrimSpace(in.RunID)
		stage := strings.TrimSpace(in.Stage)
		if runID == "" || stage == "" {
			http.Error(w, "run_id and stage are required", http.StatusBadRequest)
			return
		}
		fields := map[string]any{}
		for k, v := range in.Fields {
			fields[k] = v
		}
		if lvl := strings.TrimSpace(in.Level); lvl != "" {
			fields["level"] = lvl
		}
		if ts := strings.TrimSpace(in.Timestamp); ts != "" {
			fields["frontend_timestamp"] = ts
		}
		runSvc.TraceLogger().Append(runID, "frontend", stage, fields)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
		})
	})
	mux.HandleFunc("/debug/run-logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
		if runID == "" {
			http.Error(w, "run_id is required", http.StatusBadRequest)
			return
		}
		events, err := runSvc.TraceLogger().Read(runID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"run_id": runID,
			"events": events,
		})
	})

	// Simple CORS middleware
	h := http.Handler(mux)
	h = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, Connect-Protocol-Version, Connect-Timeout-Ms, Grpc-Timeout, X-Grpc-Web, X-User-Agent, Connect-Content-Encoding, Connect-Accept-Encoding")
			w.Header().Set("Access-Control-Expose-Headers", "Grpc-Status, Grpc-Message, Grpc-Encoding, Grpc-Accept-Encoding, Connect-Content-Encoding, Connect-Accept-Encoding")
			if r.Method == "OPTIONS" {
				return
			}
			next.ServeHTTP(w, r)
		})
	}(h)

	log.Printf("Starting API server on %s", *port)
	log.Fatal(http.ListenAndServe(*port, h2c.NewHandler(h, &http2.Server{})))
}
