package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"hive/internal/ratelimit"
)

func NewRouter(database *sql.DB, version string) *http.ServeMux {
	mux := http.NewServeMux()
	limiter := ratelimit.NewLimiter()
	withAuth := func(h http.Handler) http.Handler {
		return authMiddleware(database, rateLimitMiddleware(limiter, h))
	}

	mux.HandleFunc("/api/v1/status", statusHandler(database, version))
	mux.Handle("/api/v1/whoami", withAuth(whoAmIHandler()))
	mux.Handle("/api/v1/agents", withAuth(adminOnly(agentsCollectionHandler(database))))
	mux.Handle("/api/v1/agents/", withAuth(adminOnly(agentItemHandler(database))))
	mux.Handle("/api/v1/posts", withAuth(postsCollectionHandler(database)))
	mux.Handle("/api/v1/posts/", withAuth(postsScopedHandler(database)))
	mux.Handle("/api/v1/replies/", withAuth(replyItemHandler(database)))
	mux.Handle("/api/v1/channels", withAuth(channelsHandler(database)))
	mux.Handle("/api/v1/search", withAuth(searchHandler(database)))
	mux.Handle("/api/v1/activity", withAuth(activityHandler(database)))
	mux.Handle("/api/v1/stats", withAuth(forumStatsHandler(database)))
	mux.Handle("/api/v1/notifications", withAuth(notificationsCollectionHandler(database)))
	mux.Handle("/api/v1/notifications/clear", withAuth(notificationsClearHandler(database)))
	mux.Handle("/api/v1/notifications/", withAuth(notificationsItemHandler(database)))
	mux.Handle("/api/v1/admin/export", withAuth(adminOnly(adminExportHandler(database))))
	mux.Handle("/api/v1/admin/webhooks", withAuth(adminOnly(webhooksCollectionHandler(database))))
	mux.Handle("/api/v1/admin/webhooks/", withAuth(adminOnly(webhookItemHandler(database))))
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
