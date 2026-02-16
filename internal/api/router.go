package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"fora/internal/db"
	"fora/internal/ratelimit"
)

func NewRouter(database *sql.DB, version string) http.Handler {
	mux := http.NewServeMux()
	limiter := ratelimit.NewLimiter()
	withAuth := func(h http.Handler) http.Handler {
		return authMiddleware(database, rateLimitMiddleware(database, limiter, h))
	}

	mux.HandleFunc("/api/v1/status", statusHandler(database, version))
	mux.HandleFunc("/api/v1/primer", primerHandler())
	mux.Handle("/mcp", mcpHandler(database, version))
	mux.Handle("/api/v1/whoami", withAuth(whoAmIHandler()))
	mux.Handle("/api/v1/agents", withAuth(adminOnly(agentsCollectionHandler(database))))
	mux.Handle("/api/v1/agents/", withAuth(adminOnly(agentItemHandler(database))))
	mux.Handle("/api/v1/posts", withAuth(postsCollectionHandler(database)))
	mux.Handle("/api/v1/posts/", withAuth(postsScopedHandler(database)))
	mux.Handle("/api/v1/replies/", withAuth(replyItemHandler(database)))
	mux.Handle("/api/v1/boards", withAuth(boardsHandler(database)))
	mux.Handle("/api/v1/boards/", withAuth(boardsScopedHandler(database)))
	mux.Handle("/api/v1/search", withAuth(searchHandler(database)))
	mux.Handle("/api/v1/activity", withAuth(activityHandler(database)))
	mux.Handle("/api/v1/stats", withAuth(forumStatsHandler(database)))
	mux.Handle("/api/v1/notifications", withAuth(notificationsCollectionHandler(database)))
	mux.Handle("/api/v1/notifications/clear", withAuth(notificationsClearHandler(database)))
	mux.Handle("/api/v1/notifications/", withAuth(notificationsItemHandler(database)))
	mux.Handle("/api/v1/admin/export", withAuth(adminOnly(adminExportHandler(database))))
	mux.Handle("/api/v1/admin/webhooks", withAuth(adminOnly(webhooksCollectionHandler(database))))
	mux.Handle("/api/v1/admin/webhooks/", withAuth(adminOnly(webhookItemHandler(database))))
	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	const (
		allowOrigin  = "*"
		allowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
		allowHeaders = "Authorization, Content-Type"
		maxAge       = "86400"
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		w.Header().Set("Access-Control-Allow-Methods", allowMethods)
		w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
		w.Header().Set("Access-Control-Max-Age", maxAge)

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func statusHandler(database *sql.DB, version string) http.HandlerFunc {
	startedAt := time.Now().UTC()

	type statusHealth struct {
		Database string `json:"database"`
	}

	type statusServerStats struct {
		UptimeSeconds int64  `json:"uptime_seconds"`
		CurrentTime   string `json:"current_time"`
	}

	type statusStats struct {
		Forum  db.ForumStats     `json:"forum"`
		Server statusServerStats `json:"server"`
	}

	type statusResponse struct {
		Status    string       `json:"status"`
		Version   string       `json:"version"`
		Timestamp string       `json:"timestamp"`
		Health    statusHealth `json:"health"`
		Stats     statusStats  `json:"stats"`
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

		forumStats, err := db.GetForumStats(r.Context(), database)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load stats")
			return
		}

		now := time.Now().UTC()
		writeJSON(w, http.StatusOK, statusResponse{
			Status:    "ok",
			Version:   version,
			Timestamp: now.Format(time.RFC3339),
			Health: statusHealth{
				Database: "ok",
			},
			Stats: statusStats{
				Forum: forumStats,
				Server: statusServerStats{
					UptimeSeconds: int64(now.Sub(startedAt).Seconds()),
					CurrentTime:   now.Format(time.RFC3339),
				},
			},
		})
	}
}

func pathTail(path, prefix string) string {
	tail := strings.TrimPrefix(path, prefix)
	tail = strings.Trim(tail, "/")
	return tail
}

func postsScopedHandler(database *sql.DB) http.Handler {
	post := postItemHandler(database)
	reply := repliesHandler(database)
	thread := threadHandler(database)
	tags := postTagsHandler(database)
	status := postStatusHandler(database)
	history := postHistoryHandler(database)
	summary := postSummaryHandler(database)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/thread") {
			thread.ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/tags") {
			tags.ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/status") {
			status.ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/history") {
			history.ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/summary") {
			summary.ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/replies") {
			reply.ServeHTTP(w, r)
			return
		}
		post.ServeHTTP(w, r)
	})
}

func boardsScopedHandler(database *sql.DB) http.Handler {
	board := boardItemHandler(database)
	subscribe := boardSubscriptionHandler(database)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/subscribe") {
			subscribe.ServeHTTP(w, r)
			return
		}
		board.ServeHTTP(w, r)
	})
}

func notificationsItemHandler(database *sql.DB) http.Handler {
	markRead := notificationsMarkReadHandler(database)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/read") {
			markRead.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusNotFound, "not found")
	})
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
