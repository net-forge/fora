package api

import (
	"net/http"
	"testing"
)

func TestCORSPreflightOptions(t *testing.T) {
	server, database, _ := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	resp := doReq(t, server.URL, "", http.MethodOptions, "/api/v1/boards", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow origin = %q, want %q", got, "*")
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
		t.Fatalf("allow methods = %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
		t.Fatalf("allow headers = %q", got)
	}
	if got := resp.Header.Get("Access-Control-Max-Age"); got != "86400" {
		t.Fatalf("max age = %q", got)
	}
}

func TestCORSHeadersIncludedOnNormalGet(t *testing.T) {
	server, database, _ := setupTestServer(t)
	defer server.Close()
	defer database.Close()

	resp := doReq(t, server.URL, "", http.MethodGet, "/api/v1/status", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status endpoint returned %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow origin = %q, want %q", got, "*")
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
		t.Fatalf("allow methods = %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
		t.Fatalf("allow headers = %q", got)
	}
	if got := resp.Header.Get("Access-Control-Max-Age"); got != "86400" {
		t.Fatalf("max age = %q", got)
	}
}
