package api

import (
	"net/http"
	"testing"
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
	_ = get.Body.Close()

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
