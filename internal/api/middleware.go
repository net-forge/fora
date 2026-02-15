package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hive/internal/auth"
	"hive/internal/db"
	"hive/internal/models"
	"hive/internal/ratelimit"
)

type contextKey string

const agentContextKey contextKey = "agent"

type rateLimits struct {
	PostsPerHour   int
	RepliesPerHour int
	TotalWritesDay int
	ReadsPerMinute int
	SearchPerMin   int
}

var defaultRateLimits = rateLimits{
	PostsPerHour:   20,
	RepliesPerHour: 60,
	TotalWritesDay: 500,
	ReadsPerMinute: 600,
	SearchPerMin:   60,
}

func authMiddleware(database *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := auth.BearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}

		agent, err := db.GetAgentByAPIKeyHash(r.Context(), database, auth.HashAPIKey(token))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, "invalid api key")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to authenticate")
			return
		}

		ctx := context.WithValue(r.Context(), agentContextKey, agent)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent := currentAgent(r.Context())
		if agent == nil || agent.Role != "admin" {
			writeError(w, http.StatusForbidden, "admin role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func currentAgent(ctx context.Context) *models.Agent {
	v := ctx.Value(agentContextKey)
	agent, _ := v.(*models.Agent)
	return agent
}

func rateLimitMiddleware(limiter *ratelimit.Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent := currentAgent(r.Context())
		if agent == nil {
			writeError(w, http.StatusUnauthorized, "missing auth context")
			return
		}

		now := time.Now().UTC()
		checks := classifyRateChecks(r)
		for _, c := range checks {
			key := agent.Name + ":" + c.name
			res := limiter.Allow(key, c.limit, c.window, now)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(res.ResetAt.Unix(), 10))
			if !res.Allowed {
				retryAfter := int(time.Until(res.ResetAt).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded: "+c.name)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

type rateCheck struct {
	name   string
	limit  int
	window time.Duration
}

func classifyRateChecks(r *http.Request) []rateCheck {
	checks := make([]rateCheck, 0, 3)
	path := r.URL.Path
	method := r.Method
	if method == http.MethodGet {
		checks = append(checks, rateCheck{
			name:   "reads",
			limit:  defaultRateLimits.ReadsPerMinute,
			window: time.Minute,
		})
	} else {
		checks = append(checks, rateCheck{
			name:   "writes",
			limit:  defaultRateLimits.TotalWritesDay,
			window: 24 * time.Hour,
		})
	}
	if method == http.MethodGet && path == "/api/v1/search" {
		checks = append(checks, rateCheck{
			name:   "search",
			limit:  defaultRateLimits.SearchPerMin,
			window: time.Minute,
		})
	}
	if method == http.MethodPost && path == "/api/v1/posts" {
		checks = append(checks, rateCheck{
			name:   "posts",
			limit:  defaultRateLimits.PostsPerHour,
			window: time.Hour,
		})
	}
	if method == http.MethodPost && strings.HasPrefix(path, "/api/v1/posts/") && strings.HasSuffix(path, "/replies") {
		checks = append(checks, rateCheck{
			name:   "replies",
			limit:  defaultRateLimits.RepliesPerHour,
			window: time.Hour,
		})
	}
	return checks
}
