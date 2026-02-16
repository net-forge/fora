package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathPrefersNearestLocalConfig(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)

	projectRoot := filepath.Join(root, "project")
	deep := filepath.Join(projectRoot, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}

	parentCfg := filepath.Join(projectRoot, ".fora", "config.json")
	if err := os.MkdirAll(filepath.Dir(parentCfg), 0o755); err != nil {
		t.Fatalf("mkdir parent cfg dir: %v", err)
	}
	if err := os.WriteFile(parentCfg, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatalf("write parent cfg: %v", err)
	}

	nearCfg := filepath.Join(projectRoot, "a", ".fora", "config.json")
	if err := os.MkdirAll(filepath.Dir(nearCfg), 0o755); err != nil {
		t.Fatalf("mkdir near cfg dir: %v", err)
	}
	if err := os.WriteFile(nearCfg, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatalf("write near cfg: %v", err)
	}

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(prev) }()
	if err := os.Chdir(deep); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	if got != nearCfg {
		t.Fatalf("Path() = %q, want %q", got, nearCfg)
	}
}

func TestPathFallsBackToHomeWhenNoLocalConfig(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)

	wd := filepath.Join(root, "work", "x", "y")
	if err := os.MkdirAll(wd, 0o755); err != nil {
		t.Fatalf("mkdir wd: %v", err)
	}

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(prev) }()
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	want := filepath.Join(home, ".fora", "config.json")
	if got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestLoadExpandsEnvPlaceholders(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(home, ".fora"), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("FORA_API_KEY", "fora_ak_from_env")
	t.Setenv("FORA_API_HOST", "http://localhost")
	t.Setenv("FORA_API_PORT", "8080")

	cfgPath := filepath.Join(home, ".fora", "config.json")
	cfgJSON := `{
  "version": 1,
  "default_server": "main",
  "servers": {
    "main": {
      "url": "${FORA_API_HOST}:${FORA_API_PORT}",
      "api_key": "${FORA_API_KEY}",
      "agent": "env-agent"
    }
  }
}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(prev) }()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	srv, ok := cfg.Servers["main"]
	if !ok {
		t.Fatalf("missing main server")
	}
	if srv.URL != "http://localhost:8080" {
		t.Fatalf("url = %q, want %q", srv.URL, "http://localhost:8080")
	}
	if srv.APIKey != "fora_ak_from_env" {
		t.Fatalf("api_key = %q, want %q", srv.APIKey, "fora_ak_from_env")
	}
}
