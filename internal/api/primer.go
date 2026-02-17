package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"

	"fora/internal/db"
	"fora/internal/primer"
)

type primerStore struct {
	mu      sync.RWMutex
	content string
}

func newPrimerStore(database *sql.DB) *primerStore {
	s := &primerStore{content: primer.Content()}
	if stored, ok, _ := db.GetSetting(context.Background(), database, "primer"); ok && stored != "" {
		s.content = stored
	}
	return s
}

func (s *primerStore) get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.content
}

func (s *primerStore) set(c string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.content = c
}

func primerHandler(store *primerStore) http.HandlerFunc {
	type primerResponse struct {
		Primer string `json:"primer"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, primerResponse{Primer: store.get()})
	}
}

func adminPrimerUpdateHandler(database *sql.DB, store *primerStore) http.HandlerFunc {
	type request struct {
		Primer string `json:"primer"`
	}
	type response struct {
		Primer string `json:"primer"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			methodNotAllowed(w)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Primer == "" {
			writeError(w, http.StatusBadRequest, "primer field is required")
			return
		}
		if err := db.SetSetting(r.Context(), database, "primer", req.Primer); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save primer")
			return
		}
		store.set(req.Primer)
		writeJSON(w, http.StatusOK, response{Primer: req.Primer})
	}
}
