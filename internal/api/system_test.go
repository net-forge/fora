package api

import (
	"net/http"
	"testing"
)

func TestStatusAndWhoAmI(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	postResp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title":    "Status",
		"body":     "Body",
		"board_id": "general",
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

	status := doReq(t, server.URL, "", http.MethodGet, "/api/v1/status", nil)
	if status.StatusCode != http.StatusOK {
		t.Fatalf("status endpoint returned %d", status.StatusCode)
	}
	var statusPayload struct {
		Status    string `json:"status"`
		Version   string `json:"version"`
		Timestamp string `json:"timestamp"`
		Health    struct {
			Database string `json:"database"`
		} `json:"health"`
		Stats struct {
			Forum struct {
				Threads int `json:"threads"`
				Replies int `json:"replies"`
			} `json:"forum"`
			Server struct {
				UptimeSeconds int64  `json:"uptime_seconds"`
				CurrentTime   string `json:"current_time"`
			} `json:"server"`
		} `json:"stats"`
	}
	decodeJSON(t, status, &statusPayload)
	if statusPayload.Status == "" || statusPayload.Version == "" || statusPayload.Timestamp == "" {
		t.Fatalf("missing legacy status fields: %+v", statusPayload)
	}
	if statusPayload.Health.Database != "ok" {
		t.Fatalf("unexpected health payload: %+v", statusPayload.Health)
	}
	if statusPayload.Stats.Forum.Threads < 1 || statusPayload.Stats.Forum.Replies < 1 {
		t.Fatalf("unexpected forum stats payload: %+v", statusPayload.Stats.Forum)
	}
	if statusPayload.Stats.Server.UptimeSeconds < 0 || statusPayload.Stats.Server.CurrentTime == "" {
		t.Fatalf("unexpected server stats payload: %+v", statusPayload.Stats.Server)
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
		"title":    "Stats",
		"body":     "Body",
		"board_id": "general",
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
