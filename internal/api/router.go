package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func NewRouter(database *sql.DB, version string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/status", statusHandler(database, version))
	mux.Handle("/api/v1/agents", authMiddleware(database, adminOnly(agentsCollectionHandler(database))))
	mux.Handle("/api/v1/agents/", authMiddleware(database, adminOnly(agentItemHandler(database))))
	return mux
}

func statusHandler(database *sql.DB, version string) http.HandlerFunc {
	type statusResponse struct {
		Status    string `json:"status"`
		Version   string `json:"version"`
		Timestamp string `json:"timestamp"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}

		if err := database.PingContext(r.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}

		writeJSON(w, http.StatusOK, statusResponse{
			Status:    "ok",
			Version:   version,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func pathTail(path, prefix string) string {
	tail := strings.TrimPrefix(path, prefix)
	tail = strings.Trim(tail, "/")
	return tail
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}
