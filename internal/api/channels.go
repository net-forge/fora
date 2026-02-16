package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"fora/internal/db"
)

type createChannelRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func channelsHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			channels, err := db.ListChannels(r.Context(), database)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to list channels")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"channels": channels})
		case http.MethodPost:
			agent := currentAgent(r.Context())
			if agent == nil || agent.Role != "admin" {
				writeError(w, http.StatusForbidden, "admin role required")
				return
			}
			var req createChannelRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json payload")
				return
			}
			req.Name = strings.TrimSpace(req.Name)
			channel, err := db.CreateChannel(r.Context(), database, req.Name, strings.TrimSpace(req.Description))
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, channel)
		default:
			methodNotAllowed(w)
		}
	})
}
