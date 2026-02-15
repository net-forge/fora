package api

import (
	"net/http"
	"testing"
)

func TestAdminExportEndpoint(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	userKey := createAgentForTest(t, database, "exp-user", "agent")
	postResp := doReq(t, server.URL, userKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "Export me",
		"body":  "content body",
	})
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("create post status = %d", postResp.StatusCode)
	}
	post := decodeContent(t, postResp)

	jsonExport := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/admin/export", map[string]any{
		"format":    "json",
		"thread_id": post.ID,
	})
	if jsonExport.StatusCode != http.StatusOK {
		t.Fatalf("json export status = %d", jsonExport.StatusCode)
	}
	var jsonPayload map[string]any
	decodeJSON(t, jsonExport, &jsonPayload)
	if jsonPayload["format"] != "json" {
		t.Fatalf("unexpected json export format field")
	}

	mdExport := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/admin/export", map[string]any{
		"format":    "markdown",
		"thread_id": post.ID,
	})
	if mdExport.StatusCode != http.StatusOK {
		t.Fatalf("markdown export status = %d", mdExport.StatusCode)
	}
	var mdPayload map[string]any
	decodeJSON(t, mdExport, &mdPayload)
	if mdPayload["format"] != "markdown" {
		t.Fatalf("unexpected markdown export format field")
	}

	forbidden := doReq(t, server.URL, userKey, http.MethodPost, "/api/v1/admin/export", map[string]any{
		"format": "json",
	})
	if forbidden.StatusCode != http.StatusForbidden {
		t.Fatalf("expected non-admin export forbidden, got %d", forbidden.StatusCode)
	}
	_ = forbidden.Body.Close()
}
