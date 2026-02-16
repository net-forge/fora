package api

import (
	"database/sql"
	"net/http"
	"strings"

	"fora/internal/db"
)

func activityHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}

		limit, offset := parseLimitOffset(r)
		params := db.ListActivityParams{
			Limit:  limit,
			Offset: offset,
			Author: strings.TrimSpace(r.URL.Query().Get("author")),
		}
		events, err := db.ListActivity(r.Context(), database, params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list activity")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"activity": events,
			"limit":    limit,
			"offset":   offset,
		})
	})
}
