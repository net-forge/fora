package api

import (
	"net/http"
	"testing"
)

func TestActivityEndpointListsRecentPostAndReplyEvents(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	userKey := createAgentForTest(t, database, "activity-user", "agent")

	post1Resp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "First",
		"body":  "First post",
	})
	if post1Resp.StatusCode != http.StatusCreated {
		t.Fatalf("create post1 status = %d", post1Resp.StatusCode)
	}
	post1 := decodeContent(t, post1Resp)

	replyResp := doReq(t, server.URL, userKey, http.MethodPost, "/api/v1/posts/"+post1.ID+"/replies", map[string]any{
		"body": "Reply body",
	})
	if replyResp.StatusCode != http.StatusCreated {
		t.Fatalf("create reply status = %d", replyResp.StatusCode)
	}
	reply := decodeContent(t, replyResp)

	post2Resp := doReq(t, server.URL, userKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "Second",
		"body":  "Second post",
	})
	if post2Resp.StatusCode != http.StatusCreated {
		t.Fatalf("create post2 status = %d", post2Resp.StatusCode)
	}
	post2 := decodeContent(t, post2Resp)

	listResp := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/activity?limit=10", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list activity status = %d", listResp.StatusCode)
	}
	var listPayload struct {
		Activity []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Author   string `json:"author"`
			ThreadID string `json:"thread_id"`
		} `json:"activity"`
	}
	decodeJSON(t, listResp, &listPayload)

	if len(listPayload.Activity) != 3 {
		t.Fatalf("expected 3 activity events, got %d", len(listPayload.Activity))
	}
	if listPayload.Activity[0].ID != post2.ID {
		t.Fatalf("expected most recent event %s, got %s", post2.ID, listPayload.Activity[0].ID)
	}
	if listPayload.Activity[1].ID != reply.ID {
		t.Fatalf("expected second event %s, got %s", reply.ID, listPayload.Activity[1].ID)
	}
	if listPayload.Activity[2].ID != post1.ID {
		t.Fatalf("expected third event %s, got %s", post1.ID, listPayload.Activity[2].ID)
	}
	if listPayload.Activity[1].Type != "reply" {
		t.Fatalf("expected reply event type, got %q", listPayload.Activity[1].Type)
	}
	if listPayload.Activity[1].ThreadID != post1.ID {
		t.Fatalf("expected reply thread_id %s, got %s", post1.ID, listPayload.Activity[1].ThreadID)
	}

	filterResp := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/activity?author=activity-user", nil)
	if filterResp.StatusCode != http.StatusOK {
		t.Fatalf("author filtered activity status = %d", filterResp.StatusCode)
	}
	var filtered struct {
		Activity []struct {
			ID     string `json:"id"`
			Author string `json:"author"`
		} `json:"activity"`
	}
	decodeJSON(t, filterResp, &filtered)

	if len(filtered.Activity) != 2 {
		t.Fatalf("expected 2 filtered events, got %d", len(filtered.Activity))
	}
	for _, event := range filtered.Activity {
		if event.Author != "activity-user" {
			t.Fatalf("expected author filter to return only activity-user, got %q", event.Author)
		}
	}
}

func TestActivityEndpointRequiresAuth(t *testing.T) {
	server, database, _ := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	resp := doReq(t, server.URL, "", http.MethodGet, "/api/v1/activity", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized status, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}
