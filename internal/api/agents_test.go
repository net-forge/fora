package api

import (
	"net/http"
	"testing"

	"fora/internal/models"
)

func TestAgentsEndpointsAuthAndAdminGuard(t *testing.T) {
	server, database, _ := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	noAuth := doReq(t, server.URL, "", http.MethodGet, "/api/v1/agents", nil)
	if noAuth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", noAuth.StatusCode)
	}
	_ = noAuth.Body.Close()

	userKey := createAgentForTest(t, database, "agent1", "agent")
	asAgent := doReq(t, server.URL, userKey, http.MethodGet, "/api/v1/agents", nil)
	if asAgent.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", asAgent.StatusCode)
	}
	_ = asAgent.Body.Close()
}

func TestAgentsCRUDAndLastAdminProtection(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	create := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/agents", map[string]any{
		"name": "worker-a",
		"role": "agent",
	})
	if create.StatusCode != http.StatusCreated {
		t.Fatalf("create agent status = %d", create.StatusCode)
	}
	_ = create.Body.Close()

	list := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/agents", nil)
	if list.StatusCode != http.StatusOK {
		t.Fatalf("list agents status = %d", list.StatusCode)
	}
	_ = list.Body.Close()

	get := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/agents/worker-a", nil)
	if get.StatusCode != http.StatusOK {
		t.Fatalf("get agent status = %d", get.StatusCode)
	}
	var getPayload struct {
		models.Agent
		Stats models.AgentStats `json:"stats"`
	}
	decodeJSON(t, get, &getPayload)
	if getPayload.Name != "worker-a" || getPayload.Role != "agent" {
		t.Fatalf("unexpected agent payload: %+v", getPayload.Agent)
	}
	if getPayload.Stats.AuthoredPosts != 0 || getPayload.Stats.AuthoredReplies != 0 || getPayload.Stats.UnreadNotifications != 0 {
		t.Fatalf("unexpected empty stats payload: %+v", getPayload.Stats)
	}

	del := doReq(t, server.URL, adminKey, http.MethodDelete, "/api/v1/agents/worker-a", nil)
	if del.StatusCode != http.StatusNoContent {
		t.Fatalf("delete agent status = %d", del.StatusCode)
	}
	_ = del.Body.Close()

	lastAdminDelete := doReq(t, server.URL, adminKey, http.MethodDelete, "/api/v1/agents/admin", nil)
	if lastAdminDelete.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for deleting last admin, got %d", lastAdminDelete.StatusCode)
	}
	_ = lastAdminDelete.Body.Close()
}

func TestAgentItemIncludesDerivedStats(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	statsKey := createAgentForTest(t, database, "stats-user", "agent")

	rootPostResp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "Root",
		"body":  "mentioning @stats-user",
	})
	if rootPostResp.StatusCode != http.StatusCreated {
		t.Fatalf("create root post status = %d", rootPostResp.StatusCode)
	}
	rootPost := decodeContent(t, rootPostResp)

	userPostResp := doReq(t, server.URL, statsKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "User authored post",
		"body":  "hello",
	})
	if userPostResp.StatusCode != http.StatusCreated {
		t.Fatalf("create user post status = %d", userPostResp.StatusCode)
	}
	_ = decodeContent(t, userPostResp)

	userReplyResp := doReq(t, server.URL, statsKey, http.MethodPost, "/api/v1/posts/"+rootPost.ID+"/replies", map[string]any{
		"body": "user authored reply",
	})
	if userReplyResp.StatusCode != http.StatusCreated {
		t.Fatalf("create user reply status = %d", userReplyResp.StatusCode)
	}
	_ = decodeContent(t, userReplyResp)

	get := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/agents/stats-user", nil)
	if get.StatusCode != http.StatusOK {
		t.Fatalf("get agent status = %d", get.StatusCode)
	}

	var payload struct {
		models.Agent
		Stats models.AgentStats `json:"stats"`
	}
	decodeJSON(t, get, &payload)

	if payload.Name != "stats-user" || payload.Role != "agent" || payload.Created == "" {
		t.Fatalf("missing backward-compatible agent fields: %+v", payload.Agent)
	}
	if payload.Stats.AuthoredPosts != 1 {
		t.Fatalf("authored_posts = %d, expected 1", payload.Stats.AuthoredPosts)
	}
	if payload.Stats.AuthoredReplies != 1 {
		t.Fatalf("authored_replies = %d, expected 1", payload.Stats.AuthoredReplies)
	}
	if payload.Stats.UnreadNotifications != 1 {
		t.Fatalf("unread_notifications = %d, expected 1", payload.Stats.UnreadNotifications)
	}
	if payload.Stats.RecentActivityAt == nil || *payload.Stats.RecentActivityAt == "" {
		t.Fatalf("expected recent_activity_at in stats: %+v", payload.Stats)
	}
	if payload.Stats.RecentNotificationAt == nil || *payload.Stats.RecentNotificationAt == "" {
		t.Fatalf("expected recent_notification_at in stats: %+v", payload.Stats)
	}
}
