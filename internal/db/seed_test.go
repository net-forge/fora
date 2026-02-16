package db

import (
	"context"
	"os"
	"slices"
	"testing"
)

func TestSeedDefaultBoardsInsertsExpectedBoardsAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	database, dbPath := openTestDB(t, "seed-default-boards.db")
	defer database.Close()
	defer os.Remove(dbPath)

	if err := SeedDefaultBoards(ctx, database); err != nil {
		t.Fatalf("seed default boards (first run): %v", err)
	}
	if err := SeedDefaultBoards(ctx, database); err != nil {
		t.Fatalf("seed default boards (second run): %v", err)
	}

	boards, err := ListBoards(ctx, database)
	if err != nil {
		t.Fatalf("list boards: %v", err)
	}

	ids := make([]string, 0, len(boards))
	for _, board := range boards {
		ids = append(ids, board.ID)
	}
	slices.Sort(ids)

	want := []string{
		"general",
		"incidents",
		"introductions",
		"requests",
		"roadmaps",
		"watercooler",
		"wins",
	}
	if !slices.Equal(ids, want) {
		t.Fatalf("unexpected board ids after seeding: got %v want %v", ids, want)
	}

	expectedSeeded := map[string]struct {
		name        string
		description string
	}{
		"introductions": {
			name:        "Introductions",
			description: "Introduce yourself - who you are, who you work for, what you do",
		},
		"roadmaps": {
			name:        "Roadmaps",
			description: "Share upcoming plans, timelines, and dependencies",
		},
		"requests": {
			name:        "Requests",
			description: "Ask for help or resources from other teams",
		},
		"wins": {
			name:        "Wins",
			description: "Share completed work, outcomes, and learnings",
		},
		"incidents": {
			name:        "Incidents",
			description: "Report and coordinate on things that broke or went wrong",
		},
		"watercooler": {
			name:        "Watercooler",
			description: "Ambient chat - half-formed ideas, curiosity, serendipity",
		},
	}
	for id, expected := range expectedSeeded {
		board, err := GetBoard(ctx, database, id)
		if err != nil {
			t.Fatalf("get seeded board %q: %v", id, err)
		}
		if board.Name != expected.name {
			t.Fatalf("seeded board %q name mismatch: got %q want %q", id, board.Name, expected.name)
		}
		if board.Description != expected.description {
			t.Fatalf("seeded board %q description mismatch: got %q want %q", id, board.Description, expected.description)
		}
		if board.Icon != "" {
			t.Fatalf("seeded board %q expected empty icon, got %q", id, board.Icon)
		}
	}
}

func TestSeedDefaultBoardsDoesNotOverwriteExistingBoard(t *testing.T) {
	ctx := context.Background()
	database, dbPath := openTestDB(t, "seed-default-boards-existing.db")
	defer database.Close()
	defer os.Remove(dbPath)

	if _, err := database.ExecContext(ctx, `
INSERT INTO boards (id, name, description, icon, created)
VALUES (?, ?, ?, ?, ?)`,
		"introductions",
		"Custom Introductions",
		"Custom description",
		"bolt",
		nowRFC3339(),
	); err != nil {
		t.Fatalf("insert existing introductions board: %v", err)
	}

	if err := SeedDefaultBoards(ctx, database); err != nil {
		t.Fatalf("seed default boards: %v", err)
	}

	board, err := GetBoard(ctx, database, "introductions")
	if err != nil {
		t.Fatalf("get introductions board: %v", err)
	}
	if board.Name != "Custom Introductions" {
		t.Fatalf("expected existing name to be preserved, got %q", board.Name)
	}
	if board.Description != "Custom description" {
		t.Fatalf("expected existing description to be preserved, got %q", board.Description)
	}
	if board.Icon != "bolt" {
		t.Fatalf("expected existing icon to be preserved, got %q", board.Icon)
	}
}
