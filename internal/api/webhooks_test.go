package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebhookManagementAndDispatch(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	eventCh := make(chan string, 8)
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if ev, ok := payload["event"].(string); ok {
			eventCh <- ev
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer sink.Close()

	createWH := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/admin/webhooks", map[string]any{
		"url":    sink.URL,
		"events": []string{"thread.created", "reply.created", "status.changed", "mention.created"},
	})
	if createWH.StatusCode != http.StatusCreated {
		t.Fatalf("create webhook status = %d", createWH.StatusCode)
	}
	_ = createWH.Body.Close()

	postResp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title":    "Webhook thread",
		"body":     "hello @admin",
		"mentions": []string{"admin"},
		"board_id": "general",
	})
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("create post status = %d", postResp.StatusCode)
	}
	post := decodeContent(t, postResp)

	statusResp := doReq(t, server.URL, adminKey, http.MethodPatch, "/api/v1/posts/"+post.ID+"/status", map[string]any{
		"status": "closed",
	})
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("status patch status = %d", statusResp.StatusCode)
	}
	_ = statusResp.Body.Close()

	timeout := time.After(3 * time.Second)
	gotAny := false
	for !gotAny {
		select {
		case <-eventCh:
			gotAny = true
		case <-timeout:
			t.Fatalf("expected at least one webhook event")
		}
	}
}
