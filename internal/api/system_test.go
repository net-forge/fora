package api

import (
	"net/http"
	"testing"
)

func TestStatusAndWhoAmI(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	status := doReq(t, server.URL, "", http.MethodGet, "/api/v1/status", nil)
	if status.StatusCode != http.StatusOK {
		t.Fatalf("status endpoint returned %d", status.StatusCode)
	}
	_ = status.Body.Close()

	whoUnauthorized := doReq(t, server.URL, "", http.MethodGet, "/api/v1/whoami", nil)
	if whoUnauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("whoami without auth returned %d", whoUnauthorized.StatusCode)
	}
	_ = whoUnauthorized.Body.Close()

	who := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/whoami", nil)
	if who.StatusCode != http.StatusOK {
		t.Fatalf("whoami with auth returned %d", who.StatusCode)
	}
	_ = who.Body.Close()
}

func TestForumStatsEndpoint(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	postResp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "Stats",
		"body":  "Body",
	})
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("create post status = %d", postResp.StatusCode)
	}
	post := decodeContent(t, postResp)

	replyResp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts/"+post.ID+"/replies", map[string]any{
		"body": "reply",
	})
	if replyResp.StatusCode != http.StatusCreated {
		t.Fatalf("create reply status = %d", replyResp.StatusCode)
	}
	_ = replyResp.Body.Close()

	statsResp := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/stats", nil)
	if statsResp.StatusCode != http.StatusOK {
		t.Fatalf("stats status = %d", statsResp.StatusCode)
	}
	var payload struct {
		Stats struct {
			Threads int `json:"threads"`
			Replies int `json:"replies"`
		} `json:"stats"`
	}
	decodeJSON(t, statsResp, &payload)
	if payload.Stats.Threads < 1 || payload.Stats.Replies < 1 {
		t.Fatalf("unexpected stats payload: %+v", payload.Stats)
	}
}
