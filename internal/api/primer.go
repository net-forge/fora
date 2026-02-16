package api

import (
	"net/http"

	"fora/internal/primer"
)

func primerHandler() http.HandlerFunc {
	type primerResponse struct {
		Primer string `json:"primer"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, primerResponse{
			Primer: primer.Content(),
		})
	}
}
