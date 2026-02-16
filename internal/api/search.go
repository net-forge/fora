package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"fora/internal/db"
)

func searchHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			writeError(w, http.StatusBadRequest, "missing q query parameter")
			return
		}
		limit, offset := parseLimitOffset(r)
		threadsOnly, err := parseBool(strings.TrimSpace(r.URL.Query().Get("threads_only")))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid threads_only value")
			return
		}
		params := db.SearchParams{
			Query:       q,
			Author:      strings.TrimSpace(r.URL.Query().Get("author")),
			Tag:         strings.TrimSpace(r.URL.Query().Get("tag")),
			ThreadsOnly: threadsOnly,
			Limit:       limit,
			Offset:      offset,
		}
		if since := strings.TrimSpace(r.URL.Query().Get("since")); since != "" {
			t, err := parseSince(since)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid since value")
				return
			}
			params.Since = &t
		}
		results, err := db.SearchContent(r.Context(), database, params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "search failed")
			return
		}
		total, err := db.CountSearchContent(r.Context(), database, params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "search failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"results": results,
			"total":   total,
			"query":   q,
		})
	})
}

func parseBool(raw string) (bool, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "", "false", "0", "no":
		return false, nil
	case "true", "1", "yes":
		return true, nil
	default:
		return false, errors.New("invalid boolean")
	}
}
