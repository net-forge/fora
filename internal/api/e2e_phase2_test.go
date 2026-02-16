package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPhase2CollaborationFlowE2E(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	agentKey := createAgentForTest(t, database, "agent-c", "agent")

	postResp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title":    "Auth redesign",
		"body":     "Please review this @agent-c",
		"tags":     []string{"auth"},
		"board_id": "general",
	})
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("create post status = %d", postResp.StatusCode)
	}
	post := decodeContent(t, postResp)

	agentNotifs := doReq(t, server.URL, agentKey, http.MethodGet, "/api/v1/notifications", nil)
	if agentNotifs.StatusCode != http.StatusOK {
		t.Fatalf("agent notifications status = %d", agentNotifs.StatusCode)
	}
	_ = agentNotifs.Body.Close()

	replyResp := doReq(t, server.URL, agentKey, http.MethodPost, "/api/v1/posts/"+post.ID+"/replies", map[string]any{
		"body": "I suggest splitting auth and session concerns",
	})
	if replyResp.StatusCode != http.StatusCreated {
		t.Fatalf("reply status = %d", replyResp.StatusCode)
	}
	_ = replyResp.Body.Close()

	adminNotifs := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/notifications", nil)
	if adminNotifs.StatusCode != http.StatusOK {
		t.Fatalf("admin notifications status = %d", adminNotifs.StatusCode)
	}
	var notifPayload struct {
		Notifications []struct {
			Type string `json:"type"`
		} `json:"notifications"`
	}
	decodeJSON(t, adminNotifs, &notifPayload)
	foundReply := false
	for _, n := range notifPayload.Notifications {
		if n.Type == "reply" {
			foundReply = true
			break
		}
	}
	if !foundReply {
		t.Fatalf("expected reply notification")
	}

	searchResp := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/search?q=session", nil)
	if searchResp.StatusCode != http.StatusOK {
		t.Fatalf("search status = %d", searchResp.StatusCode)
	}
	var searchPayload struct {
		Results []map[string]any `json:"results"`
	}
	decodeJSON(t, searchResp, &searchPayload)
	if len(searchPayload.Results) == 0 {
		t.Fatalf("expected search results")
	}

	rawThread := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/posts/"+post.ID+"/thread?format=raw", nil)
	if rawThread.StatusCode != http.StatusOK {
		t.Fatalf("raw thread status = %d", rawThread.StatusCode)
	}
	rawBytes, _ := io.ReadAll(rawThread.Body)
	_ = rawThread.Body.Close()
	rawText := string(rawBytes)
	if !strings.Contains(rawText, "Auth redesign") || !strings.Contains(rawText, "splitting auth") {
		t.Fatalf("raw thread missing expected content")
	}
}
