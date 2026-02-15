package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"hive/internal/auth"
)

func TestImportJSONRoundTrip(t *testing.T) {
	ctx := context.Background()
	srcDB, srcPath := openTestDB(t, "src.db")
	defer srcDB.Close()
	defer os.Remove(srcPath)

	apiKey, _ := auth.GenerateAPIKey()
	if err := CreateAgent(ctx, srcDB, "alice", "admin", auth.HashAPIKey(apiKey), nil); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	post, err := CreatePost(ctx, srcDB, "alice", strPtr("hello"), "body text", []string{"tag1"}, nil, nil)
	if err != nil {
		t.Fatalf("create post: %v", err)
	}
	if _, err := CreateReply(ctx, srcDB, "alice", post.ID, "reply body", nil); err != nil {
		t.Fatalf("create reply: %v", err)
	}

	exported, err := ExportJSON(ctx, srcDB, ExportOptions{})
	if err != nil {
		t.Fatalf("export json: %v", err)
	}
	exportPath := filepath.Join(t.TempDir(), "export.json")
	b, _ := json.Marshal(exported)
	if err := os.WriteFile(exportPath, b, 0o644); err != nil {
		t.Fatalf("write export file: %v", err)
	}

	dstDB, dstPath := openTestDB(t, "dst.db")
	defer dstDB.Close()
	defer os.Remove(dstPath)
	if err := ImportJSON(ctx, dstDB, exportPath); err != nil {
		t.Fatalf("import json: %v", err)
	}

	var count int
	if err := dstDB.QueryRowContext(ctx, `SELECT COUNT(1) FROM content`).Scan(&count); err != nil {
		t.Fatalf("count content: %v", err)
	}
	if count < 2 {
		t.Fatalf("expected imported content >=2, got %d", count)
	}
}

func TestImportMarkdownRoundTrip(t *testing.T) {
	ctx := context.Background()
	srcDB, srcPath := openTestDB(t, "src-md.db")
	defer srcDB.Close()
	defer os.Remove(srcPath)

	apiKey, _ := auth.GenerateAPIKey()
	if err := CreateAgent(ctx, srcDB, "bob", "admin", auth.HashAPIKey(apiKey), nil); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	post, err := CreatePost(ctx, srcDB, "bob", strPtr("md"), "body md", []string{"docs"}, nil, nil)
	if err != nil {
		t.Fatalf("create post: %v", err)
	}
	if _, err := CreateReply(ctx, srcDB, "bob", post.ID, "reply md", nil); err != nil {
		t.Fatalf("create reply: %v", err)
	}

	files, err := ExportMarkdown(ctx, srcDB, ExportOptions{})
	if err != nil {
		t.Fatalf("export markdown: %v", err)
	}
	exportDir := filepath.Join(t.TempDir(), "md-export")
	for _, f := range files {
		target := filepath.Join(exportDir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(target, []byte(f.Content), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	dstDB, dstPath := openTestDB(t, "dst-md.db")
	defer dstDB.Close()
	defer os.Remove(dstPath)
	if err := ImportMarkdown(ctx, dstDB, exportDir); err != nil {
		t.Fatalf("import markdown: %v", err)
	}

	var count int
	if err := dstDB.QueryRowContext(ctx, `SELECT COUNT(1) FROM content`).Scan(&count); err != nil {
		t.Fatalf("count content: %v", err)
	}
	if count < 2 {
		t.Fatalf("expected imported content >=2, got %d", count)
	}
}

func openTestDB(t *testing.T, name string) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	database, err := Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := ApplyMigrations(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database, path
}

func strPtr(v string) *string { return &v }
