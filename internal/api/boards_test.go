package api

import (
	"net/http"
	"slices"
	"testing"
)

func TestBoardsCRUDAndPostFiltering(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	userKey := createAgentForTest(t, database, "board-user", "agent")

	create := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/boards", map[string]any{
		"name":        "Engineering",
		"description": "Build and platform",
		"icon":        "wrench",
		"tags":        []string{"eng", "backend"},
	})
	if create.StatusCode != http.StatusCreated {
		t.Fatalf("create board status = %d", create.StatusCode)
	}
	var board struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Icon        string   `json:"icon"`
		Tags        []string `json:"tags"`
	}
	decodeJSON(t, create, &board)
	if board.ID == "" {
		t.Fatalf("expected board id")
	}
	if board.Name != "Engineering" {
		t.Fatalf("unexpected board name: %q", board.Name)
	}
	if board.Description != "Build and platform" {
		t.Fatalf("unexpected board description: %q", board.Description)
	}
	if board.Icon != "wrench" {
		t.Fatalf("unexpected board icon: %q", board.Icon)
	}
	if !slices.Equal(board.Tags, []string{"eng", "backend"}) {
		t.Fatalf("unexpected board tags: %v", board.Tags)
	}

	list := doReq(t, server.URL, userKey, http.MethodGet, "/api/v1/boards", nil)
	if list.StatusCode != http.StatusOK {
		t.Fatalf("list boards status = %d", list.StatusCode)
	}
	_ = list.Body.Close()

	item := doReq(t, server.URL, userKey, http.MethodGet, "/api/v1/boards/"+board.ID, nil)
	if item.StatusCode != http.StatusOK {
		t.Fatalf("get board status = %d", item.StatusCode)
	}
	var itemBoard struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Icon        string   `json:"icon"`
		Tags        []string `json:"tags"`
	}
	decodeJSON(t, item, &itemBoard)
	if itemBoard.ID != board.ID {
		t.Fatalf("unexpected board id from item endpoint: got %q want %q", itemBoard.ID, board.ID)
	}
	if itemBoard.Name != board.Name || itemBoard.Description != board.Description || itemBoard.Icon != board.Icon {
		t.Fatalf("unexpected board details from item endpoint: %+v", itemBoard)
	}
	gotTags := append([]string(nil), itemBoard.Tags...)
	wantTags := append([]string(nil), board.Tags...)
	slices.Sort(gotTags)
	slices.Sort(wantTags)
	if !slices.Equal(gotTags, wantTags) {
		t.Fatalf("unexpected board tags from item endpoint: got %v want %v", itemBoard.Tags, board.Tags)
	}

	postResp := doReq(t, server.URL, userKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title":    "Board post",
		"body":     "content",
		"board_id": board.ID,
	})
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("create board post status = %d", postResp.StatusCode)
	}
	_ = postResp.Body.Close()

	filtered := doReq(t, server.URL, userKey, http.MethodGet, "/api/v1/posts?board="+board.ID, nil)
	if filtered.StatusCode != http.StatusOK {
		t.Fatalf("filter by board status = %d", filtered.StatusCode)
	}
	var payload struct {
		Threads []any `json:"threads"`
	}
	decodeJSON(t, filtered, &payload)
	if len(payload.Threads) == 0 {
		t.Fatalf("expected at least one board-filtered thread")
	}
}

func TestBoardSubscribeUnsubscribe(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	userKey := createAgentForTest(t, database, "board-subscriber", "agent")

	create := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/boards", map[string]any{
		"name": "Announcements",
	})
	if create.StatusCode != http.StatusCreated {
		t.Fatalf("create board status = %d", create.StatusCode)
	}
	var board struct {
		ID string `json:"id"`
	}
	decodeJSON(t, create, &board)

	sub := doReq(t, server.URL, userKey, http.MethodPost, "/api/v1/boards/"+board.ID+"/subscribe", nil)
	if sub.StatusCode != http.StatusOK {
		t.Fatalf("subscribe status = %d", sub.StatusCode)
	}
	_ = sub.Body.Close()

	unsub := doReq(t, server.URL, userKey, http.MethodDelete, "/api/v1/boards/"+board.ID+"/subscribe", nil)
	if unsub.StatusCode != http.StatusNoContent {
		t.Fatalf("unsubscribe status = %d", unsub.StatusCode)
	}
	_ = unsub.Body.Close()
}
