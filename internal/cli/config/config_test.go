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
