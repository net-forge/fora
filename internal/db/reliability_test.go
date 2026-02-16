package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"fora/internal/auth"
)

func TestExportImportFidelityCounts(t *testing.T) {
	ctx := context.Background()
	srcDB, srcPath := openTestDB(t, "fidelity-src.db")
	defer srcDB.Close()
	defer os.Remove(srcPath)

	adminKey, _ := auth.GenerateAPIKey()
	if err := CreateAgent(ctx, srcDB, "admin-f", "admin", auth.HashAPIKey(adminKey), nil); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	agentKey, _ := auth.GenerateAPIKey()
	if err := CreateAgent(ctx, srcDB, "agent-f", "agent", auth.HashAPIKey(agentKey), nil); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	post, err := CreatePost(ctx, srcDB, "admin-f", strPtr("Fidelity"), "hello @agent-f", []string{"ops"}, []string{"agent-f"}, "general")
	if err != nil {
		t.Fatalf("create post: %v", err)
	}
	if _, err := CreateReply(ctx, srcDB, "agent-f", post.ID, "ack", nil); err != nil {
		t.Fatalf("create reply: %v", err)
	}

	exported, err := ExportJSON(ctx, srcDB, ExportOptions{})
	if err != nil {
		t.Fatalf("export json: %v", err)
	}
	exportPath := filepath.Join(t.TempDir(), "fidelity.json")
	b, _ := json.Marshal(exported)
	if err := os.WriteFile(exportPath, b, 0o644); err != nil {
		t.Fatalf("write export file: %v", err)
	}

	dstDB, dstPath := openTestDB(t, "fidelity-dst.db")
	defer dstDB.Close()
	defer os.Remove(dstPath)
	if err := ImportJSON(ctx, dstDB, exportPath); err != nil {
		t.Fatalf("import json: %v", err)
	}

	assertCountEqual(t, ctx, srcDB, dstDB, "content")
	assertCountEqual(t, ctx, srcDB, dstDB, "tags")
	assertCountEqual(t, ctx, srcDB, dstDB, "mentions")
	assertCountEqual(t, ctx, srcDB, dstDB, "notifications")
}

func assertCountEqual(t *testing.T, ctx context.Context, a, b *sql.DB, table string) {
	t.Helper()
	var ca, cb int
	if err := a.QueryRowContext(ctx, `SELECT COUNT(1) FROM `+table).Scan(&ca); err != nil {
		t.Fatalf("count %s in src: %v", table, err)
	}
	if err := b.QueryRowContext(ctx, `SELECT COUNT(1) FROM `+table).Scan(&cb); err != nil {
		t.Fatalf("count %s in dst: %v", table, err)
	}
	if ca != cb {
		t.Fatalf("count mismatch for %s: src=%d dst=%d", table, ca, cb)
	}
}
