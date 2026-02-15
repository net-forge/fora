package api

import (
	"net/http"
	"testing"
)

func TestChannelsCRUDAndPostFiltering(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	userKey := createAgentForTest(t, database, "channel-user", "agent")

	create := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/channels", map[string]any{
		"name":        "Engineering",
		"description": "Build and platform",
	})
	if create.StatusCode != http.StatusCreated {
		t.Fatalf("create channel status = %d", create.StatusCode)
	}
	var channel struct {
		ID string `json:"id"`
	}
	decodeJSON(t, create, &channel)
	if channel.ID == "" {
		t.Fatalf("expected channel id")
	}

	list := doReq(t, server.URL, userKey, http.MethodGet, "/api/v1/channels", nil)
	if list.StatusCode != http.StatusOK {
		t.Fatalf("list channels status = %d", list.StatusCode)
	}
	_ = list.Body.Close()

	postResp := doReq(t, server.URL, userKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title":      "Channel post",
		"body":       "content",
		"channel_id": channel.ID,
	})
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("create channel post status = %d", postResp.StatusCode)
	}
	_ = postResp.Body.Close()

	filtered := doReq(t, server.URL, userKey, http.MethodGet, "/api/v1/posts?channel="+channel.ID, nil)
	if filtered.StatusCode != http.StatusOK {
		t.Fatalf("filter by channel status = %d", filtered.StatusCode)
	}
	var payload struct {
		Threads []any `json:"threads"`
	}
	decodeJSON(t, filtered, &payload)
	if len(payload.Threads) == 0 {
		t.Fatalf("expected at least one channel-filtered thread")
	}
}
