package webhook

import (
	"encoding/json"
	"net/http"
	"time"
)

type HealthHandler struct {
	serviceName string
	version     string
	startedAt   time.Time
}

func NewHealthHandler(serviceName string, version string, startedAt time.Time) *HealthHandler {
	return &HealthHandler{
		serviceName: serviceName,
		version:     version,
		startedAt:   startedAt,
	}
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"service":   h.serviceName,
		"version":   h.version,
		"uptime":    time.Since(h.startedAt).Round(time.Second).String(),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
