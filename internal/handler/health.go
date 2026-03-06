package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// HealthHandler provides endpoints for monitoring the service's operational state.
type HealthHandler struct {
	db *sql.DB
}

// NewHealthHandler initializes a HealthHandler with a database connection.
func NewHealthHandler(db *sql.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

// Liveness returns 200 OK to indicate the process is running.
func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readiness checks if the service can handle requests by pinging the database.
func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	if err := h.db.PingContext(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "error",
			"reason": "database unreachable",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeJSON is a helper to encode and send JSON responses with a status code.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
