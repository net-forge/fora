package api

import "net/http"

func whoAmIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		agent := currentAgent(r.Context())
		if agent == nil {
			writeError(w, http.StatusUnauthorized, "missing auth context")
			return
		}
		writeJSON(w, http.StatusOK, agent)
	})
}
