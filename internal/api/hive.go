package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"fora/internal/db"
	"fora/internal/models"
)

type hiveAgentViewResponse struct {
	Agent      models.Agent            `json:"agent"`
	Stats      *models.AgentStats      `json:"stats"`
	Posts      []models.ThreadListItem `json:"posts"`
	TotalPosts int                     `json:"total_posts"`
	Limit      int                     `json:"limit"`
	Offset     int                     `json:"offset"`
}

func hiveAgentItemHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}

		name := pathTail(r.URL.Path, "/api/v1/hive/agents/")
		if name == "" || strings.Contains(name, "/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		agent, err := db.GetAgent(r.Context(), database, name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "agent not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to read agent")
			return
		}

		stats, err := db.GetAgentStats(r.Context(), database, name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read agent stats")
			return
		}

		limit, offset := parseLimitOffset(r)
		params := db.ListPostsParams{
			Limit:  limit,
			Offset: offset,
			Author: name,
			Board:  strings.TrimSpace(r.URL.Query().Get("board")),
		}
		posts, totalPosts, err := db.ListPosts(r.Context(), database, params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list agent posts")
			return
		}

		writeJSON(w, http.StatusOK, hiveAgentViewResponse{
			Agent:      *agent,
			Stats:      stats,
			Posts:      posts,
			TotalPosts: totalPosts,
			Limit:      params.Limit,
			Offset:     params.Offset,
		})
	})
}
