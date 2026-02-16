package db

import (
	"context"
	"os"
	"slices"
	"testing"
)

func TestCreateListGetAndUpdateBoards(t *testing.T) {
	ctx := context.Background()
	database, dbPath := openTestDB(t, "boards.db")
	defer database.Close()
	defer os.Remove(dbPath)

	created, err := CreateBoard(ctx, database, "Engineering", "Eng discussions", "wrench", []string{"eng", "backend", "eng"})
	if err != nil {
		t.Fatalf("create board: %v", err)
	}
	if created.ID != "engineering" {
		t.Fatalf("unexpected board id: %q", created.ID)
	}
	if !slices.Equal(created.Tags, []string{"eng", "backend"}) {
		t.Fatalf("unexpected created tags: %v", created.Tags)
	}

	boards, err := ListBoards(ctx, database)
	if err != nil {
		t.Fatalf("list boards: %v", err)
	}
	if len(boards) < 2 {
		t.Fatalf("expected at least default+created boards, got %d", len(boards))
	}

	loaded, err := GetBoard(ctx, database, created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if loaded.Icon != "wrench" {
		t.Fatalf("unexpected icon: %q", loaded.Icon)
	}

	tags, err := UpdateBoardTags(ctx, database, created.ID, []string{"ops", "backend"}, []string{"eng"})
	if err != nil {
		t.Fatalf("update board tags: %v", err)
	}
	expected := []string{"backend", "ops"}
	if !slices.Equal(tags, expected) {
		t.Fatalf("unexpected board tags after update: got %v want %v", tags, expected)
	}

	exists, err := BoardExists(ctx, database, created.ID)
	if err != nil {
		t.Fatalf("board exists: %v", err)
	}
	if !exists {
		t.Fatalf("expected board %q to exist", created.ID)
	}

	stats, err := GetForumStats(ctx, database)
	if err != nil {
		t.Fatalf("forum stats: %v", err)
	}
	if stats.Boards < 2 {
		t.Fatalf("expected at least two boards in stats, got %d", stats.Boards)
	}
}

func TestBoardSubscriptionsAndPostNotifications(t *testing.T) {
	ctx := context.Background()
	database, dbPath := openTestDB(t, "board-subscriptions.db")
	defer database.Close()
	defer os.Remove(dbPath)

	if err := CreateAgent(ctx, database, "alice", "admin", "hash-alice", nil); err != nil {
		t.Fatalf("create alice: %v", err)
	}
	if err := CreateAgent(ctx, database, "bob", "agent", "hash-bob", nil); err != nil {
		t.Fatalf("create bob: %v", err)
	}

	if err := SubscribeToBoard(ctx, database, "general", "bob"); err != nil {
		t.Fatalf("subscribe bob: %v", err)
	}
	subs, err := ListBoardSubscribers(ctx, database, "general")
	if err != nil {
		t.Fatalf("list board subscribers: %v", err)
	}
	if !slices.Contains(subs, "bob") {
		t.Fatalf("expected bob in general subscribers: %v", subs)
	}

	if _, err := CreatePost(ctx, database, "alice", strPtr("hello"), "board post body", nil, nil, "general"); err != nil {
		t.Fatalf("create board post: %v", err)
	}

	notifs, err := ListNotifications(ctx, database, "bob", true, 20, 0)
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	if len(notifs) == 0 {
		t.Fatalf("expected at least one notification for bob")
	}
	if notifs[0].Type != "board_post" {
		t.Fatalf("unexpected notification type: got %q want board_post", notifs[0].Type)
	}

	agentSubs, err := ListAgentSubscriptions(ctx, database, "bob")
	if err != nil {
		t.Fatalf("list agent subscriptions: %v", err)
	}
	if !slices.Contains(agentSubs, "general") {
		t.Fatalf("expected bob subscription to include general: %v", agentSubs)
	}

	if err := UnsubscribeFromBoard(ctx, database, "general", "bob"); err != nil {
		t.Fatalf("unsubscribe bob: %v", err)
	}
	subs, err = ListBoardSubscribers(ctx, database, "general")
	if err != nil {
		t.Fatalf("list board subscribers after unsubscribe: %v", err)
	}
	if slices.Contains(subs, "bob") {
		t.Fatalf("expected bob to be unsubscribed, got subscribers: %v", subs)
	}
}
