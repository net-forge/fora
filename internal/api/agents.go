package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"hive/internal/auth"
	"hive/internal/db"
)

var agentNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type createAgentRequest struct {
	Name     string  `json:"name"`
	Role     string  `json:"role"`
	Metadata *string `json:"metadata"`
}

type createAgentResponse struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	APIKey string `json:"api_key"`
}

func agentsCollectionHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			agents, err := db.ListAgents(r.Context(), database)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to list agents")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"agents": agents, "total": len(agents)})
		case http.MethodPost:
			var req createAgentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json payload")
				return
			}

			req.Name = strings.TrimSpace(req.Name)
			req.Role = strings.TrimSpace(req.Role)
			if req.Name == "" || !agentNamePattern.MatchString(req.Name) {
				writeError(w, http.StatusBadRequest, "invalid agent name")
				return
			}
			if req.Role == "" {
				req.Role = "agent"
			}
			if req.Role != "agent" && req.Role != "admin" {
				writeError(w, http.StatusBadRequest, "role must be 'agent' or 'admin'")
				return
			}

			apiKey, err := auth.GenerateAPIKey()
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to generate api key")
				return
			}
			if err := db.CreateAgent(
				r.Context(),
				database,
				req.Name,
				req.Role,
				auth.HashAPIKey(apiKey),
				req.Metadata,
			); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "constraint") {
					writeError(w, http.StatusConflict, "agent already exists")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to create agent")
				return
			}

			writeJSON(w, http.StatusCreated, createAgentResponse{
				Name:   req.Name,
				Role:   req.Role,
				APIKey: apiKey,
			})
		default:
			methodNotAllowed(w)
		}
	})
}

func agentItemHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := pathTail(r.URL.Path, "/api/v1/agents/")
		if name == "" {
			writeError(w, http.StatusBadRequest, "missing agent name")
			return
		}

		switch r.Method {
		case http.MethodGet:
			agent, err := db.GetAgent(r.Context(), database, name)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "agent not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to read agent")
				return
			}
			writeJSON(w, http.StatusOK, agent)
		case http.MethodDelete:
			agent, err := db.GetAgent(r.Context(), database, name)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "agent not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to read agent")
				return
			}
			if agent.Role == "admin" {
				adminCount, err := db.CountAdmins(r.Context(), database)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "failed to validate admin deletion")
					return
				}
				if adminCount <= 1 {
					writeError(w, http.StatusConflict, "cannot delete the last admin")
					return
				}
			}

			if err := db.DeleteAgent(r.Context(), database, name); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "agent not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to delete agent")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			methodNotAllowed(w)
		}
	})
}
