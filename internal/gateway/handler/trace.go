package handler

import (
	"encoding/json"
	gatewayworker "insightify/internal/gateway/service/worker"
	"net/http"
	"strings"
)

type TraceHandler struct {
	workerSvc *gatewayworker.Service
}

func NewTraceHandler(workerSvc *gatewayworker.Service) *TraceHandler {
	return &TraceHandler{workerSvc: workerSvc}
}

func (h *TraceHandler) HandleFrontendTrace(w http.ResponseWriter, r *http.Request) {
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
	h.workerSvc.Telemetry().Append(runID, "frontend", stage, fields)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true,
	})
}

func (h *TraceHandler) HandleRunLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
	if runID == "" {
		http.Error(w, "run_id is required", http.StatusBadRequest)
		return
	}
	events, err := h.workerSvc.Telemetry().Read(runID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"run_id": runID,
		"events": events,
	})
}
