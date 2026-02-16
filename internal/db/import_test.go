package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fora/internal/auth"
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
	productBoard, err := CreateBoard(ctx, srcDB, "Product", "Product roadmap and planning", "rocket", []string{"roadmap", "planning"})
	if err != nil {
		t.Fatalf("create board: %v", err)
	}
	post, err := CreatePost(ctx, srcDB, "alice", strPtr("hello"), "body text", []string{"tag1"}, nil, productBoard.ID)
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
	containsProductBoard := false
	for _, b := range exported.Boards {
		if b.ID != productBoard.ID {
			continue
		}
		containsProductBoard = true
		if b.Icon != "rocket" {
			t.Fatalf("unexpected exported board icon: %q", b.Icon)
		}
		if !slices.Equal(b.Tags, []string{"planning", "roadmap"}) {
			t.Fatalf("unexpected exported board tags: %v", b.Tags)
		}
	}
	if !containsProductBoard {
		t.Fatalf("exported JSON missing board %q", productBoard.ID)
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

	importedBoard, err := GetBoard(ctx, dstDB, productBoard.ID)
	if err != nil {
		t.Fatalf("get imported board: %v", err)
	}
	if importedBoard.Icon != "rocket" {
		t.Fatalf("unexpected imported board icon: %q", importedBoard.Icon)
	}
	if !slices.Equal(importedBoard.Tags, []string{"planning", "roadmap"}) {
		t.Fatalf("unexpected imported board tags: %v", importedBoard.Tags)
	}

	var importedPostBoardID string
	if err := dstDB.QueryRowContext(ctx, `SELECT board_id FROM content WHERE id = ?`, post.ID).Scan(&importedPostBoardID); err != nil {
		t.Fatalf("load imported post board_id: %v", err)
	}
	if importedPostBoardID != productBoard.ID {
		t.Fatalf("unexpected imported post board_id: got %q want %q", importedPostBoardID, productBoard.ID)
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
	post, err := CreatePost(ctx, srcDB, "bob", strPtr("md"), "body md", []string{"docs"}, nil, "general")
	if err != nil {
		t.Fatalf("create post: %v", err)
	}
	reply1, err := CreateReply(ctx, srcDB, "bob", post.ID, "reply md", nil)
	if err != nil {
		t.Fatalf("create reply: %v", err)
	}
	if _, err := CreateReply(ctx, srcDB, "bob", reply1.ID, "child reply md", nil); err != nil {
		t.Fatalf("create child reply: %v", err)
	}

	files, err := ExportMarkdown(ctx, srcDB, ExportOptions{})
	if err != nil {
		t.Fatalf("export markdown: %v", err)
	}
	filesByPath := make(map[string]string, len(files))
	for _, f := range files {
		filesByPath[f.Path] = f.Content
	}
	expectedPaths := []string{
		fmt.Sprintf("threads/%s/post.md", post.ID),
		fmt.Sprintf("threads/%s/replies/%s/reply.md", post.ID, reply1.ID),
	}
	for _, path := range expectedPaths {
		if _, ok := filesByPath[path]; !ok {
			t.Fatalf("expected markdown export path %q", path)
		}
	}
	var childReplyPath string
	for path := range maps.Keys(filesByPath) {
		if strings.HasPrefix(path, fmt.Sprintf("threads/%s/replies/%s/replies/", post.ID, reply1.ID)) && strings.HasSuffix(path, "/reply.md") {
			childReplyPath = path
			break
		}
	}
	if childReplyPath == "" {
		t.Fatalf("expected nested child reply path under parent reply")
	}
	if strings.Contains(filesByPath[expectedPaths[0]], "{\n") {
		t.Fatalf("expected YAML frontmatter for post export, found JSON-looking object")
	}
	if !strings.Contains(filesByPath[expectedPaths[0]], "id: "+post.ID) {
		t.Fatalf("expected YAML key-value style frontmatter in post export")
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

func TestImportMarkdownAcceptsYAMLAndJSONFrontmatter(t *testing.T) {
	ctx := context.Background()
	importDir := filepath.Join(t.TempDir(), "mixed-md-import")

	postID := "post-yaml-1"
	replyID := "reply-yaml-1"
	legacyReplyID := "reply-json-legacy-1"
	now := "2026-02-15T00:00:00Z"

	postPath := filepath.Join(importDir, "threads", postID, "post.md")
	if err := os.MkdirAll(filepath.Dir(postPath), 0o755); err != nil {
		t.Fatalf("mkdir post path: %v", err)
	}
	postContent := strings.Join([]string{
		"---",
		"id: " + postID,
		"type: post",
		"author: yaml-author",
		"title: YAML post",
		"created: " + now,
		"updated: " + now,
		"thread_id: " + postID,
		"status: open",
		"tags:",
		"  - docs",
		"---",
		"yaml post body",
		"",
	}, "\n")
	if err := os.WriteFile(postPath, []byte(postContent), 0o644); err != nil {
		t.Fatalf("write post file: %v", err)
	}

	replyPath := filepath.Join(importDir, "threads", postID, "replies", replyID, "reply.md")
	if err := os.MkdirAll(filepath.Dir(replyPath), 0o755); err != nil {
		t.Fatalf("mkdir reply path: %v", err)
	}
	replyContent := strings.Join([]string{
		"---",
		"id: " + replyID,
		"type: reply",
		"author: yaml-author",
		"created: " + now,
		"updated: " + now,
		"thread_id: " + postID,
		"parent_id: " + postID,
		"status: open",
		"---",
		"yaml reply body",
		"",
	}, "\n")
	if err := os.WriteFile(replyPath, []byte(replyContent), 0o644); err != nil {
		t.Fatalf("write reply file: %v", err)
	}

	legacyPath := filepath.Join(importDir, "threads", postID, "replies", legacyReplyID+".md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy path: %v", err)
	}
	legacyFrontmatter := map[string]any{
		"id":        legacyReplyID,
		"type":      "reply",
		"author":    "json-author",
		"created":   now,
		"updated":   now,
		"thread_id": postID,
		"parent_id": replyID,
		"status":    "open",
	}
	legacyJSON, err := json.MarshalIndent(legacyFrontmatter, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy frontmatter: %v", err)
	}
	legacyContent := fmt.Sprintf("---\n%s\n---\nlegacy json frontmatter reply\n", legacyJSON)
	if err := os.WriteFile(legacyPath, []byte(legacyContent), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	dstDB, dstPath := openTestDB(t, "dst-mixed-md.db")
	defer dstDB.Close()
	defer os.Remove(dstPath)
	if err := ImportMarkdown(ctx, dstDB, importDir); err != nil {
		t.Fatalf("import markdown mixed formats: %v", err)
	}

	var ids []string
	rows, err := dstDB.QueryContext(ctx, `SELECT id FROM content ORDER BY id ASC`)
	if err != nil {
		t.Fatalf("query imported ids: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan imported id: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate imported ids: %v", err)
	}
	expectedIDs := []string{postID, legacyReplyID, replyID}
	slices.Sort(expectedIDs)
	slices.Sort(ids)
	if !slices.Equal(ids, expectedIDs) {
		t.Fatalf("unexpected imported ids: got %v want %v", ids, expectedIDs)
	}

	var (
		threadID string
		parentID sql.NullString
	)
	if err := dstDB.QueryRowContext(ctx, `SELECT thread_id, parent_id FROM content WHERE id = ?`, legacyReplyID).Scan(&threadID, &parentID); err != nil {
		t.Fatalf("load imported legacy reply: %v", err)
	}
	if threadID != postID {
		t.Fatalf("unexpected thread_id for legacy reply: got %q want %q", threadID, postID)
	}
	if !parentID.Valid || parentID.String != replyID {
		t.Fatalf("unexpected parent_id for legacy reply: got %q want %q", parentID.String, replyID)
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
