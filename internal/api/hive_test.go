package api

import (
	"net/http"
	"testing"

	"fora/internal/models"
)

func TestHiveAgentViewRequiresAuth(t *testing.T) {
	server, database, _ := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	resp := doReq(t, server.URL, "", http.MethodGet, "/api/v1/hive/agents/admin", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestHiveAgentViewReturnsInfoAndPostsForAgents(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	aliceKey := createAgentForTest(t, database, "alice-hive", "agent")
	bobKey := createAgentForTest(t, database, "bob-hive", "agent")

	for _, body := range []string{"post one", "post two"} {
		create := doReq(t, server.URL, aliceKey, http.MethodPost, "/api/v1/posts", map[string]any{
			"title":    "Alice post",
			"body":     body,
			"board_id": "general",
		})
		if create.StatusCode != http.StatusCreated {
			t.Fatalf("create post status = %d", create.StatusCode)
		}
		_ = decodeContent(t, create)
	}

	view := doReq(t, server.URL, bobKey, http.MethodGet, "/api/v1/hive/agents/alice-hive?limit=1", nil)
	if view.StatusCode != http.StatusOK {
		t.Fatalf("view status = %d", view.StatusCode)
	}
	var payload struct {
		Agent      models.Agent            `json:"agent"`
		Stats      models.AgentStats       `json:"stats"`
		Posts      []models.ThreadListItem `json:"posts"`
		TotalPosts int                     `json:"total_posts"`
		Limit      int                     `json:"limit"`
		Offset     int                     `json:"offset"`
	}
	decodeJSON(t, view, &payload)

	if payload.Agent.Name != "alice-hive" {
		t.Fatalf("unexpected agent name %q", payload.Agent.Name)
	}
	if payload.Stats.AuthoredPosts != 2 {
		t.Fatalf("authored_posts = %d, expected 2", payload.Stats.AuthoredPosts)
	}
	if payload.TotalPosts != 2 {
		t.Fatalf("total_posts = %d, expected 2", payload.TotalPosts)
	}
	if payload.Limit != 1 || payload.Offset != 0 {
		t.Fatalf("unexpected paging values: limit=%d offset=%d", payload.Limit, payload.Offset)
	}
	if len(payload.Posts) != 1 {
		t.Fatalf("expected 1 post in page, got %d", len(payload.Posts))
	}

	missing := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/hive/agents/does-not-exist", nil)
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing agent, got %d", missing.StatusCode)
	}
	_ = missing.Body.Close()
}
