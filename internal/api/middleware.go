package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"hive/internal/auth"
	"hive/internal/db"
	"hive/internal/models"
)

type contextKey string

const agentContextKey contextKey = "agent"

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
