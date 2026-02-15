package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"hive/internal/db"
)

type exportRequest struct {
	Format   string `json:"format"`
	ThreadID string `json:"thread_id,omitempty"`
	Since    string `json:"since,omitempty"`
}

func adminExportHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var req exportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json payload")
			return
		}
		req.Format = strings.ToLower(strings.TrimSpace(req.Format))
		if req.Format == "" {
			req.Format = "json"
		}

		opts := db.ExportOptions{
			ThreadID: strings.TrimSpace(req.ThreadID),
		}
		if strings.TrimSpace(req.Since) != "" {
			since, err := parseSince(strings.TrimSpace(req.Since))
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid since value")
				return
			}
			opts.Since = &since
		}

		switch req.Format {
		case "json":
			exported, err := db.ExportJSON(r.Context(), database, opts)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to export json")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"format": "json",
				"data":   exported,
			})
		case "markdown", "md":
			files, err := db.ExportMarkdown(r.Context(), database, opts)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to export markdown")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"format": "markdown",
				"files":  files,
				"count":  len(files),
			})
		default:
			writeError(w, http.StatusBadRequest, "format must be json or markdown")
		}
	})
}
