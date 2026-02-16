package api

import (
	"net/http"
	"testing"
)

func TestSearchEndpoint(t *testing.T) {
	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	userKey := createAgentForTest(t, database, "searcher", "agent")

	p1Resp := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title":    "Auth Flow",
		"body":     "The authentication flow needs review",
		"tags":     []string{"security"},
		"board_id": "general",
	})
	if p1Resp.StatusCode != http.StatusCreated {
		t.Fatalf("create p1 status = %d", p1Resp.StatusCode)
	}
	p1 := decodeContent(t, p1Resp)

	p2Resp := doReq(t, server.URL, userKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title":    "Deploy Notes",
		"body":     "Deployment checklist for staging",
		"tags":     []string{"ops"},
		"board_id": "general",
	})
	if p2Resp.StatusCode != http.StatusCreated {
		t.Fatalf("create p2 status = %d", p2Resp.StatusCode)
	}
	_ = p2Resp.Body.Close()

	r1Resp := doReq(t, server.URL, userKey, http.MethodPost, "/api/v1/posts/"+p1.ID+"/replies", map[string]any{
		"body": "Authentication retries should be bounded",
	})
	if r1Resp.StatusCode != http.StatusCreated {
		t.Fatalf("create reply status = %d", r1Resp.StatusCode)
	}
	_ = r1Resp.Body.Close()

	q1 := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/search?q=authentication", nil)
	if q1.StatusCode != http.StatusOK {
		t.Fatalf("search status = %d", q1.StatusCode)
	}
	var all struct {
		Results []map[string]any `json:"results"`
		Total   int              `json:"total"`
	}
	decodeJSON(t, q1, &all)
	if len(all.Results) < 2 {
		t.Fatalf("expected >=2 search results, got %d", len(all.Results))
	}
	if all.Total != len(all.Results) {
		t.Fatalf("expected total to match unpaginated result count, got total=%d results=%d", all.Total, len(all.Results))
	}

	qPaginated := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/search?q=authentication&limit=1&offset=1", nil)
	if qPaginated.StatusCode != http.StatusOK {
		t.Fatalf("paginated search status = %d", qPaginated.StatusCode)
	}
	var paginated struct {
		Results []map[string]any `json:"results"`
		Total   int              `json:"total"`
	}
	decodeJSON(t, qPaginated, &paginated)
	if len(paginated.Results) != 1 {
		t.Fatalf("expected 1 paginated result, got %d", len(paginated.Results))
	}
	if paginated.Total != all.Total {
		t.Fatalf("expected paginated total=%d to match unpaginated total=%d", paginated.Total, all.Total)
	}
	if paginated.Total <= len(paginated.Results) {
		t.Fatalf("expected paginated total > returned results, got total=%d results=%d", paginated.Total, len(paginated.Results))
	}

	qThreadsOnly := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/search?q=authentication&threads_only=true", nil)
	if qThreadsOnly.StatusCode != http.StatusOK {
		t.Fatalf("search threads-only status = %d", qThreadsOnly.StatusCode)
	}
	var threadsOnly struct {
		Results []map[string]any `json:"results"`
	}
	decodeJSON(t, qThreadsOnly, &threadsOnly)
	for _, r := range threadsOnly.Results {
		if typ, _ := r["type"].(string); typ != "post" {
			t.Fatalf("threads_only returned non-post result: %v", r)
		}
	}

	qAuthor := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/search?q=deployment&author=searcher", nil)
	if qAuthor.StatusCode != http.StatusOK {
		t.Fatalf("search author filter status = %d", qAuthor.StatusCode)
	}
	var byAuthor struct {
		Results []map[string]any `json:"results"`
	}
	decodeJSON(t, qAuthor, &byAuthor)
	if len(byAuthor.Results) != 1 {
		t.Fatalf("expected 1 author-filtered result, got %d", len(byAuthor.Results))
	}

	qBoard := doReq(t, server.URL, adminKey, http.MethodGet, "/api/v1/search?q=authentication&board=general", nil)
	if qBoard.StatusCode != http.StatusOK {
		t.Fatalf("search board filter status = %d", qBoard.StatusCode)
	}
}
