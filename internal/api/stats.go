package api

import (
	"database/sql"
	"net/http"

	"hive/internal/db"
)

func forumStatsHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		stats, err := db.GetForumStats(r.Context(), database)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load stats")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"stats": stats,
		})
	})
}
