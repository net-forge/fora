package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"fora/internal/db"
)

func notificationsCollectionHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		agent := currentAgent(r.Context())
		if agent == nil {
			writeError(w, http.StatusUnauthorized, "missing auth context")
			return
		}
		limit, offset := parseLimitOffset(r)
		includeRead := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("all")), "true")
		items, err := db.ListNotifications(r.Context(), database, agent.Name, includeRead, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list notifications")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"notifications": items,
			"limit":         limit,
			"offset":        offset,
		})
	})
}

func notificationsMarkReadHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			methodNotAllowed(w)
			return
		}
		agent := currentAgent(r.Context())
		if agent == nil {
			writeError(w, http.StatusUnauthorized, "missing auth context")
			return
		}
		id := pathTail(r.URL.Path, "/api/v1/notifications/")
		id = strings.TrimSuffix(id, "/read")
		id = strings.Trim(id, "/")
		if id == "" {
			writeError(w, http.StatusBadRequest, "missing notification id")
			return
		}
		if err := db.MarkNotificationRead(r.Context(), database, agent.Name, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "notification not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to mark notification as read")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

func notificationsClearHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		agent := currentAgent(r.Context())
		if agent == nil {
			writeError(w, http.StatusUnauthorized, "missing auth context")
			return
		}
		count, err := db.MarkAllNotificationsRead(r.Context(), database, agent.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to clear notifications")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":        "ok",
			"updated_count": count,
		})
	})
}
