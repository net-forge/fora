package api

import (
	"net/http"
	"testing"
)

func TestRateLimitOnPostCreation(t *testing.T) {
	orig := defaultRateLimits
	defaultRateLimits = rateLimits{
		PostsPerHour:   1,
		RepliesPerHour: 10,
		TotalWritesDay: 100,
		ReadsPerMinute: 100,
		SearchPerMin:   100,
	}
	defer func() { defaultRateLimits = orig }()

	server, database, adminKey := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	first := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "one",
		"body":  "first",
	})
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("expected first post 201, got %d", first.StatusCode)
	}
	_ = first.Body.Close()

	second := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "two",
		"body":  "second",
	})
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected second post 429, got %d", second.StatusCode)
	}
	if second.Header.Get("X-RateLimit-Limit") == "" || second.Header.Get("Retry-After") == "" {
		t.Fatalf("expected rate limit headers to be present")
	}
	_ = second.Body.Close()
}
