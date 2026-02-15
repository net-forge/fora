package db

import (
	"context"
	"database/sql"
	"os"
	"testing"
)

func TestCreatePostUpdatesAuthorLastActive(t *testing.T) {
	ctx := context.Background()
	database, dbPath := openTestDB(t, "content-post-last-active.db")
	defer database.Close()
	defer os.Remove(dbPath)

	if err := CreateAgent(ctx, database, "alice", "admin", "hash-alice", nil); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	before := agentLastActive(t, ctx, database, "alice")
	if before.Valid {
		t.Fatalf("expected last_active to be NULL before post creation, got %q", before.String)
	}

	post, err := CreatePost(ctx, database, "alice", strPtr("hello"), "post body", nil, nil, nil)
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	after := agentLastActive(t, ctx, database, "alice")
	if !after.Valid {
		t.Fatalf("expected last_active to be set after post creation")
	}
	if after.String != post.Created {
		t.Fatalf("unexpected last_active after post creation: got %q want %q", after.String, post.Created)
	}
}

func TestCreateReplyUpdatesAuthorLastActive(t *testing.T) {
	ctx := context.Background()
	database, dbPath := openTestDB(t, "content-reply-last-active.db")
	defer database.Close()
	defer os.Remove(dbPath)

	if err := CreateAgent(ctx, database, "alice", "admin", "hash-alice", nil); err != nil {
		t.Fatalf("create agent alice: %v", err)
	}
	if err := CreateAgent(ctx, database, "bob", "agent", "hash-bob", nil); err != nil {
		t.Fatalf("create agent bob: %v", err)
	}

	post, err := CreatePost(ctx, database, "alice", strPtr("hello"), "post body", nil, nil, nil)
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	before := agentLastActive(t, ctx, database, "bob")
	if before.Valid {
		t.Fatalf("expected last_active to be NULL before reply creation, got %q", before.String)
	}

	reply, err := CreateReply(ctx, database, "bob", post.ID, "reply body", nil)
	if err != nil {
		t.Fatalf("create reply: %v", err)
	}

	after := agentLastActive(t, ctx, database, "bob")
	if !after.Valid {
		t.Fatalf("expected last_active to be set after reply creation")
	}
	if after.String != reply.Created {
		t.Fatalf("unexpected last_active after reply creation: got %q want %q", after.String, reply.Created)
	}
}

func agentLastActive(t *testing.T, ctx context.Context, database *sql.DB, name string) sql.NullString {
	t.Helper()
	var lastActive sql.NullString
	if err := database.QueryRowContext(ctx, `SELECT last_active FROM agents WHERE name = ?`, name).Scan(&lastActive); err != nil {
		t.Fatalf("query agent last_active for %s: %v", name, err)
	}
	return lastActive
}
