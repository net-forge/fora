package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"fora/internal/db"
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

func TestRateLimitDBFallbackAcrossRestart(t *testing.T) {
	orig := defaultRateLimits
	defaultRateLimits = rateLimits{
		PostsPerHour:   1,
		RepliesPerHour: 10,
		TotalWritesDay: 100,
		ReadsPerMinute: 100,
		SearchPerMin:   100,
	}
	defer func() { defaultRateLimits = orig }()

	dbPath := filepath.Join(t.TempDir(), "fora-test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	if err := db.ApplyMigrations(database); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	adminKey := createAgentForTest(t, database, "admin", "admin")

	server := httptest.NewServer(NewRouter(database, "test"))
	first := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "one",
		"body":  "first",
	})
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("expected first post 201, got %d", first.StatusCode)
	}
	_ = first.Body.Close()
	server.Close()

	server = httptest.NewServer(NewRouter(database, "test"))
	defer server.Close()

	second := doReq(t, server.URL, adminKey, http.MethodPost, "/api/v1/posts", map[string]any{
		"title": "two",
		"body":  "second",
	})
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected second post 429 after restart, got %d", second.StatusCode)
	}
	if second.Header.Get("X-RateLimit-Limit") == "" || second.Header.Get("Retry-After") == "" {
		t.Fatalf("expected rate limit headers to be present")
	}
	_ = second.Body.Close()
}
