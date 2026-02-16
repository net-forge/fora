package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdPrimerPrintsPrimerMarkdown(t *testing.T) {
	const (
		apiKey = "fora_ak_test"
		body   = "# Welcome to Fora\n\nPrimer body.\n"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/primer" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Fatalf("unexpected auth header: %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"primer": body})
	}))
	defer srv.Close()

	writeCLIConfig(t, srv.URL, apiKey)

	output, err := captureStdout(t, cmdPrimer)
	if err != nil {
		t.Fatalf("cmdPrimer returned error: %v", err)
	}
	if output != body {
		t.Fatalf("unexpected primer output: got %q want %q", output, body)
	}
}

func TestCmdPrimerRequiresConnection(t *testing.T) {
	setCLIEnv(t)

	err := cmdPrimer()
	if err == nil {
		t.Fatalf("expected error when not connected")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func setCLIEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})
	return home
}

func writeCLIConfig(t *testing.T, serverURL, apiKey string) {
	t.Helper()
	home := setCLIEnv(t)
	cfgPath := filepath.Join(home, ".fora", "config.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	payload := map[string]any{
		"version":        1,
		"default_server": "main",
		"servers": map[string]any{
			"main": map[string]any{
				"url":          serverURL,
				"api_key":      apiKey,
				"connected_at": "2026-02-16T00:00:00Z",
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(cfgPath, b, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}

	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = orig

	out, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}
	return string(out), runErr
}
