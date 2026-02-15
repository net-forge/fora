package api

import (
	"net/http"
	"testing"
)

func TestNotificationsLifecycle(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	userKey := createAgentForTest(t, database, "user-b", "agent")

	postResp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title":    "Mentions",
		"body":     "hello @user-b",
		"mentions": []string{"user-b"},
	})
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("create post status = %d", postResp.StatusCode)
	}
	post := decodeContent(t, postResp)

	userNotifs := doReq(t, server.URL, userKey, http.MethodGet, "/api/v1/notifications", nil)
	if userNotifs.StatusCode != http.StatusOK {
		t.Fatalf("list notifications status = %d", userNotifs.StatusCode)
	}
	var userNotifPayload struct {
		Notifications []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"notifications"`
	}
	decodeJSON(t, userNotifs, &userNotifPayload)
	if len(userNotifPayload.Notifications) == 0 {
		t.Fatalf("expected mention notification for user-b")
	}
	if userNotifPayload.Notifications[0].Type != "mention" {
		t.Fatalf("expected mention notification, got %q", userNotifPayload.Notifications[0].Type)
	}

	markRead := doReq(t, server.URL, userKey, http.MethodPatch, "/api/v1/notifications/"+userNotifPayload.Notifications[0].ID+"/read", nil)
	if markRead.StatusCode != http.StatusOK {
		t.Fatalf("mark read status = %d", markRead.StatusCode)
	}
	_ = markRead.Body.Close()

	replyResp := doReq(t, server.URL, userKey, http.MethodPost, "/api/v1/posts/"+post.ID+"/replies", map[string]any{
		"body": "replying now",
	})
	if replyResp.StatusCode != http.StatusCreated {
		t.Fatalf("create reply status = %d", replyResp.StatusCode)
	}
	_ = replyResp.Body.Close()

	adminNotifs := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/notifications", nil)
	if adminNotifs.StatusCode != http.StatusOK {
		t.Fatalf("admin notifications status = %d", adminNotifs.StatusCode)
	}
	var adminNotifPayload struct {
		Notifications []struct {
			Type string `json:"type"`
		} `json:"notifications"`
	}
	decodeJSON(t, adminNotifs, &adminNotifPayload)
	foundReply := false
	for _, n := range adminNotifPayload.Notifications {
		if n.Type == "reply" {
			foundReply = true
			break
		}
	}
	if !foundReply {
		t.Fatalf("expected reply notification for admin")
	}

	clearResp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/notifications/clear", nil)
	if clearResp.StatusCode != http.StatusOK {
		t.Fatalf("clear notifications status = %d", clearResp.StatusCode)
	}
	_ = clearResp.Body.Close()
}
