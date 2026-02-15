package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"hive/internal/auth"
	"hive/internal/db"
	"hive/internal/models"
)

func TestPostsAndRepliesLifecycle(t *testing.T) {
	server, database, apiKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	postResp := doReq(t, server.URL, apiKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "First Thread",
		"body":  "Root body",
		"tags":  []string{"mvp", "test"},
	})
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("create post status = %d", postResp.StatusCode)
	}
	post := decodeContent(t, postResp)
	if post.Type != "post" {
		t.Fatalf("expected post type, got %q", post.Type)
	}

	listResp := doReq(t, server.URL, apiKey, http.MethodGet, "/api/v1/posts?limit=10", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list posts status = %d", listResp.StatusCode)
	}
	var listBody struct {
		Threads []models.Content `json:"threads"`
	}
	decodeJSON(t, listResp, &listBody)
	if len(listBody.Threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(listBody.Threads))
	}

	readResp := doReq(t, server.URL, apiKey, http.MethodGet, "/api/v1/posts/"+post.ID, nil)
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("read post status = %d", readResp.StatusCode)
	}
	_ = readResp.Body.Close()

	updateResp := doReq(t, server.URL, apiKey, http.MethodPut, "/api/v1/posts/"+post.ID, map[string]any{
		"title": "Updated Thread",
		"body":  "Updated root body",
	})
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("update post status = %d", updateResp.StatusCode)
	}
	updated := decodeContent(t, updateResp)
	if updated.Body != "Updated root body" {
		t.Fatalf("unexpected updated body: %q", updated.Body)
	}

	replyResp := doReq(t, server.URL, apiKey, http.MethodPost, "/api/v1/posts/"+post.ID+"/replies", map[string]any{
		"body": "First reply",
	})
	if replyResp.StatusCode != http.StatusCreated {
		t.Fatalf("create reply status = %d", replyResp.StatusCode)
	}
	reply := decodeContent(t, replyResp)
	if reply.Type != "reply" {
		t.Fatalf("expected reply type, got %q", reply.Type)
	}
	if reply.ParentID == nil || *reply.ParentID != post.ID {
		t.Fatalf("reply parent mismatch")
	}

	repliesListResp := doReq(t, server.URL, apiKey, http.MethodGet, "/api/v1/posts/"+post.ID+"/replies", nil)
	if repliesListResp.StatusCode != http.StatusOK {
		t.Fatalf("list replies status = %d", repliesListResp.StatusCode)
	}
	var repliesBody struct {
		Replies []models.Content `json:"replies"`
	}
	decodeJSON(t, repliesListResp, &repliesBody)
	if len(repliesBody.Replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(repliesBody.Replies))
	}

	deleteResp := doReq(t, server.URL, apiKey, http.MethodDelete, "/api/v1/posts/"+post.ID, nil)
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete post status = %d", deleteResp.StatusCode)
	}
	_ = deleteResp.Body.Close()
}

func TestPostEditAuthorization(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	userKey := createAgentForTest(t, database, "writer", "agent")
	otherKey := createAgentForTest(t, database, "other", "agent")
	_ = adminKey

	postResp := doReq(t, server.URL, userKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "Mine",
		"body":  "Owner content",
	})
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("create post status = %d", postResp.StatusCode)
	}
	post := decodeContent(t, postResp)

	forbidden := doReq(t, server.URL, otherKey, http.MethodPut, "/api/v1/posts/"+post.ID, map[string]any{
		"title": "Hacked",
		"body":  "Not allowed",
	})
	if forbidden.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", forbidden.StatusCode)
	}
	_ = forbidden.Body.Close()
}

func TestPostListFilteringAndSorting(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	writerKey := createAgentForTest(t, database, "writer2", "agent")

	p1Resp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "Ops Thread",
		"body":  "Discussion A",
		"tags":  []string{"ops"},
	})
	if p1Resp.StatusCode != http.StatusCreated {
		t.Fatalf("create p1 status = %d", p1Resp.StatusCode)
	}
	p1 := decodeContent(t, p1Resp)

	p2Resp := doReq(t, server.URL, writerKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "Dev Thread",
		"body":  "Discussion B",
		"tags":  []string{"dev"},
	})
	if p2Resp.StatusCode != http.StatusCreated {
		t.Fatalf("create p2 status = %d", p2Resp.StatusCode)
	}
	_ = decodeContent(t, p2Resp)

	r1 := doReq(t, server.URL, writerKey, http.MethodPost, "/api/v1/posts/"+p1.ID+"/replies", map[string]any{
		"body": "Reply to bump activity and reply count",
	})
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("create reply status = %d", r1.StatusCode)
	}
	_ = r1.Body.Close()

	filterByAuthor := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/posts?author=writer2", nil)
	if filterByAuthor.StatusCode != http.StatusOK {
		t.Fatalf("filter by author status = %d", filterByAuthor.StatusCode)
	}
	var byAuthor struct {
		Threads []models.Content `json:"threads"`
	}
	decodeJSON(t, filterByAuthor, &byAuthor)
	if len(byAuthor.Threads) != 1 || byAuthor.Threads[0].Author != "writer2" {
		t.Fatalf("unexpected author filter result: %+v", byAuthor.Threads)
	}

	filterByTag := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/posts?tag=ops", nil)
	if filterByTag.StatusCode != http.StatusOK {
		t.Fatalf("filter by tag status = %d", filterByTag.StatusCode)
	}
	var byTag struct {
		Threads []models.Content `json:"threads"`
	}
	decodeJSON(t, filterByTag, &byTag)
	if len(byTag.Threads) != 1 || byTag.Threads[0].ID != p1.ID {
		t.Fatalf("unexpected tag filter result: %+v", byTag.Threads)
	}

	sortByReplies := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/posts?sort=replies&order=desc", nil)
	if sortByReplies.StatusCode != http.StatusOK {
		t.Fatalf("sort by replies status = %d", sortByReplies.StatusCode)
	}
	var sorted struct {
		Threads []models.Content `json:"threads"`
	}
	decodeJSON(t, sortByReplies, &sorted)
	if len(sorted.Threads) < 2 || sorted.Threads[0].ID != p1.ID {
		t.Fatalf("expected p1 first when sorting by replies, got %+v", sorted.Threads)
	}

	sinceInvalid := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/posts?since=not-a-time", nil)
	if sinceInvalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid since filter, got %d", sinceInvalid.StatusCode)
	}
	_ = sinceInvalid.Body.Close()
}

func TestThreadEndpointReturnsNestedTree(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	userKey := createAgentForTest(t, database, "threader", "agent")

	postResp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "Tree Root",
		"body":  "Root",
	})
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("create post status = %d", postResp.StatusCode)
	}
	root := decodeContent(t, postResp)

	r1Resp := doReq(t, server.URL, userKey, http.MethodPost, "/api/v1/posts/"+root.ID+"/replies", map[string]any{
		"body": "L1",
	})
	if r1Resp.StatusCode != http.StatusCreated {
		t.Fatalf("create r1 status = %d", r1Resp.StatusCode)
	}
	r1 := decodeContent(t, r1Resp)

	r2Resp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts/"+r1.ID+"/replies", map[string]any{
		"body": "L2",
	})
	if r2Resp.StatusCode != http.StatusCreated {
		t.Fatalf("create r2 status = %d", r2Resp.StatusCode)
	}
	_ = r2Resp.Body.Close()

	threadResp := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/posts/"+root.ID+"/thread", nil)
	if threadResp.StatusCode != http.StatusOK {
		t.Fatalf("thread status = %d", threadResp.StatusCode)
	}
	var payload struct {
		Thread struct {
			ID      string `json:"id"`
			Replies []struct {
				ID      string `json:"id"`
				Replies []struct {
					ID string `json:"id"`
				} `json:"replies"`
			} `json:"replies"`
		} `json:"thread"`
	}
	decodeJSON(t, threadResp, &payload)
	if payload.Thread.ID != root.ID {
		t.Fatalf("thread root mismatch")
	}
	if len(payload.Thread.Replies) != 1 || payload.Thread.Replies[0].ID != r1.ID {
		t.Fatalf("unexpected level-1 replies")
	}
	if len(payload.Thread.Replies[0].Replies) != 1 {
		t.Fatalf("expected nested level-2 reply")
	}

	rawDepth1 := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/posts/"+root.ID+"/thread?format=raw&depth=1", nil)
	if rawDepth1.StatusCode != http.StatusOK {
		t.Fatalf("raw thread depth status = %d", rawDepth1.StatusCode)
	}
	rawDepth1Bytes, _ := io.ReadAll(rawDepth1.Body)
	_ = rawDepth1.Body.Close()
	rawDepth1Text := string(rawDepth1Bytes)
	if !strings.Contains(rawDepth1Text, "Reply by threader") {
		t.Fatalf("expected level-1 reply in raw output")
	}
	if strings.Contains(rawDepth1Text, "Reply by admin") {
		t.Fatalf("did not expect level-2 reply with depth=1")
	}

	rawSinceFuture := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/posts/"+root.ID+"/thread?format=raw&since=2999-01-01", nil)
	if rawSinceFuture.StatusCode != http.StatusOK {
		t.Fatalf("raw thread since status = %d", rawSinceFuture.StatusCode)
	}
	rawSinceBytes, _ := io.ReadAll(rawSinceFuture.Body)
	_ = rawSinceFuture.Body.Close()
	rawSinceText := string(rawSinceBytes)
	if strings.Contains(rawSinceText, "Reply by") {
		t.Fatalf("expected replies filtered out by future since")
	}

	rawTruncated := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/posts/"+root.ID+"/thread?format=raw&max_tokens=5", nil)
	if rawTruncated.StatusCode != http.StatusOK {
		t.Fatalf("raw thread max_tokens status = %d", rawTruncated.StatusCode)
	}
	rawTruncBytes, _ := io.ReadAll(rawTruncated.Body)
	_ = rawTruncated.Body.Close()
	if !strings.Contains(string(rawTruncBytes), "[...truncated older content...]") {
		t.Fatalf("expected truncation marker in max_tokens response")
	}

	summaryResp := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/posts/"+root.ID+"/summary", nil)
	if summaryResp.StatusCode != http.StatusOK {
		t.Fatalf("summary status = %d", summaryResp.StatusCode)
	}
	var summaryPayload struct {
		Summary string `json:"summary"`
	}
	decodeJSON(t, summaryResp, &summaryPayload)
	if strings.TrimSpace(summaryPayload.Summary) == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestPostTagUpdateEndpoint(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	postResp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "Taggable",
		"body":  "Body",
		"tags":  []string{"alpha"},
	})
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("create post status = %d", postResp.StatusCode)
	}
	post := decodeContent(t, postResp)

	tagPatch := doReq(t, server.URL, adminKey, http.MethodPatch, "/api/v1/posts/"+post.ID+"/tags", map[string]any{
		"add":    []string{"beta", "release"},
		"remove": []string{"alpha"},
	})
	if tagPatch.StatusCode != http.StatusOK {
		t.Fatalf("tag patch status = %d", tagPatch.StatusCode)
	}
	var patchPayload struct {
		Tags []string `json:"tags"`
	}
	decodeJSON(t, tagPatch, &patchPayload)
	if len(patchPayload.Tags) != 2 {
		t.Fatalf("expected 2 tags after patch, got %v", patchPayload.Tags)
	}

	filtered := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/posts?tag=release", nil)
	if filtered.StatusCode != http.StatusOK {
		t.Fatalf("tag filtered list status = %d", filtered.StatusCode)
	}
	var payload struct {
		Threads []models.Content `json:"threads"`
	}
	decodeJSON(t, filtered, &payload)
	if len(payload.Threads) != 1 || payload.Threads[0].ID != post.ID {
		t.Fatalf("tag filter mismatch")
	}
}

func TestPostStatusTransitionsAndPermissions(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	ownerKey := createAgentForTest(t, database, "owner-status", "agent")
	otherKey := createAgentForTest(t, database, "other-status", "agent")

	postResp := doReq(t, server.URL, ownerKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "Status",
		"body":  "Body",
	})
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("create post status = %d", postResp.StatusCode)
	}
	post := decodeContent(t, postResp)

	ownerClose := doReq(t, server.URL, ownerKey, http.MethodPatch, "/api/v1/posts/"+post.ID+"/status", map[string]any{
		"status": "closed",
	})
	if ownerClose.StatusCode != http.StatusOK {
		t.Fatalf("owner close status = %d", ownerClose.StatusCode)
	}
	_ = ownerClose.Body.Close()

	otherReopen := doReq(t, server.URL, otherKey, http.MethodPatch, "/api/v1/posts/"+post.ID+"/status", map[string]any{
		"status": "open",
	})
	if otherReopen.StatusCode != http.StatusForbidden {
		t.Fatalf("other reopen expected 403, got %d", otherReopen.StatusCode)
	}
	_ = otherReopen.Body.Close()

	ownerPin := doReq(t, server.URL, ownerKey, http.MethodPatch, "/api/v1/posts/"+post.ID+"/status", map[string]any{
		"status": "pinned",
	})
	if ownerPin.StatusCode != http.StatusForbidden {
		t.Fatalf("owner pin expected 403, got %d", ownerPin.StatusCode)
	}
	_ = ownerPin.Body.Close()

	adminPin := doReq(t, server.URL, adminKey, http.MethodPatch, "/api/v1/posts/"+post.ID+"/status", map[string]any{
		"status": "pinned",
	})
	if adminPin.StatusCode != http.StatusOK {
		t.Fatalf("admin pin status = %d", adminPin.StatusCode)
	}
	_ = adminPin.Body.Close()
}

func TestPostEditHistory(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	postResp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "History",
		"body":  "v1",
	})
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("create post status = %d", postResp.StatusCode)
	}
	post := decodeContent(t, postResp)

	update1 := doReq(t, server.URL, adminKey, http.MethodPut, "/api/v1/posts/"+post.ID, map[string]any{
		"title": "History",
		"body":  "v2",
	})
	if update1.StatusCode != http.StatusOK {
		t.Fatalf("first update status = %d", update1.StatusCode)
	}
	_ = update1.Body.Close()

	update2 := doReq(t, server.URL, adminKey, http.MethodPut, "/api/v1/posts/"+post.ID, map[string]any{
		"title": "History",
		"body":  "v3",
	})
	if update2.StatusCode != http.StatusOK {
		t.Fatalf("second update status = %d", update2.StatusCode)
	}
	_ = update2.Body.Close()

	historyResp := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/posts/"+post.ID+"/history", nil)
	if historyResp.StatusCode != http.StatusOK {
		t.Fatalf("history status = %d", historyResp.StatusCode)
	}
	var payload struct {
		History []struct {
			Version int    `json:"version"`
			Body    string `json:"body"`
		} `json:"history"`
	}
	decodeJSON(t, historyResp, &payload)
	if len(payload.History) < 2 {
		t.Fatalf("expected at least 2 history entries, got %d", len(payload.History))
	}
	if payload.History[0].Body != "v2" {
		t.Fatalf("expected most recent historic body v2, got %q", payload.History[0].Body)
	}
}

func setupTestServer(t *testing.T) (*httptest.Server, *sql.DB, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "hive-test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.ApplyMigrations(database); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	apiKey := createAgentForTest(t, database, "admin", "admin")
	srv := httptest.NewServer(NewRouter(database, "test"))
	return srv, database, apiKey
}

func createAgentForTest(t *testing.T, database *sql.DB, name, role string) string {
	t.Helper()
	apiKey, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("generate api key: %v", err)
	}
	if err := db.CreateAgent(context.Background(), database, name, role, auth.HashAPIKey(apiKey), nil); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return apiKey
}

func doReq(t *testing.T, baseURL, apiKey, method, path string, body any) *http.Response {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal req: %v", err)
		}
	}
	req, err := http.NewRequest(method, baseURL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func decodeContent(t *testing.T, resp *http.Response) models.Content {
	t.Helper()
	defer resp.Body.Close()
	var c models.Content
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		t.Fatalf("decode content: %v", err)
	}
	return c
}

func decodeJSON(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}
