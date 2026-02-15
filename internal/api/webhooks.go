package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"hive/internal/db"
)

type createWebhookRequest struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Secret string   `json:"secret"`
}

func webhooksCollectionHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			items, err := db.ListWebhooks(r.Context(), database, false)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to list webhooks")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"webhooks": items})
		case http.MethodPost:
			var req createWebhookRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json payload")
				return
			}
			req.URL = strings.TrimSpace(req.URL)
			if req.URL == "" {
				writeError(w, http.StatusBadRequest, "url is required")
				return
			}
			wh, err := db.CreateWebhook(r.Context(), database, req.URL, req.Events, req.Secret)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, wh)
		default:
			methodNotAllowed(w)
		}
	})
}

func webhookItemHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			methodNotAllowed(w)
			return
		}
		id := pathTail(r.URL.Path, "/api/v1/admin/webhooks/")
		if strings.TrimSpace(id) == "" {
			writeError(w, http.StatusBadRequest, "missing webhook id")
			return
		}
		if err := db.DeleteWebhook(r.Context(), database, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "webhook not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to delete webhook")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func emitWebhookEvent(database *sql.DB, eventType string, payload map[string]any) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		items, err := db.ListWebhooks(ctx, database, true)
		if err != nil {
			return
		}
		body := map[string]any{
			"event": eventType,
			"at":    time.Now().UTC().Format(time.RFC3339),
			"data":  payload,
		}
		b, err := json.Marshal(body)
		if err != nil {
			return
		}
		client := &http.Client{Timeout: 5 * time.Second}
		for _, wh := range items {
			if !eventAllowed(wh.Events, eventType) {
				continue
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.URL, bytes.NewReader(b))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			if strings.TrimSpace(wh.Secret) != "" {
				mac := hmac.New(sha256.New, []byte(wh.Secret))
				_, _ = mac.Write(b)
				req.Header.Set("X-Hive-Signature", hex.EncodeToString(mac.Sum(nil)))
			}
			_, _ = client.Do(req)
		}
	}()
}

func eventAllowed(events []string, event string) bool {
	for _, e := range events {
		if e == "*" || strings.EqualFold(strings.TrimSpace(e), event) {
			return true
		}
	}
	return false
}
