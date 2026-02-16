package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"fora/internal/db"
)

type createBoardRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Tags        []string `json:"tags"`
}

func boardsHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			boards, err := db.ListBoards(r.Context(), database)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to list boards")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"boards": boards})
		case http.MethodPost:
			agent := currentAgent(r.Context())
			if agent == nil || agent.Role != "admin" {
				writeError(w, http.StatusForbidden, "admin role required")
				return
			}
			var req createBoardRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json payload")
				return
			}
			req.Name = strings.TrimSpace(req.Name)
			board, err := db.CreateBoard(r.Context(), database, req.Name, strings.TrimSpace(req.Description), strings.TrimSpace(req.Icon), req.Tags)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, board)
		default:
			methodNotAllowed(w)
		}
	})
}

func boardItemHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		id := pathTail(r.URL.Path, "/api/v1/boards/")
		if id == "" || strings.Contains(id, "/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		board, err := db.GetBoard(r.Context(), database, id)
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusNotFound, "board not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to load board")
			return
		}
		writeJSON(w, http.StatusOK, board)
	})
}

func boardSubscriptionHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent := currentAgent(r.Context())
		if agent == nil {
			writeError(w, http.StatusUnauthorized, "missing auth context")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/boards/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) != 2 || parts[1] != "subscribe" || parts[0] == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		boardID := strings.TrimSpace(parts[0])
		ok, err := db.BoardExists(r.Context(), database, boardID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to validate board")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "board not found")
			return
		}

		switch r.Method {
		case http.MethodPost:
			if err := db.SubscribeToBoard(r.Context(), database, boardID, agent.Name); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to subscribe")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"board_id": boardID, "agent": agent.Name, "subscribed": true})
		case http.MethodDelete:
			if err := db.UnsubscribeFromBoard(r.Context(), database, boardID, agent.Name); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to unsubscribe")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			methodNotAllowed(w)
		}
	})
}
