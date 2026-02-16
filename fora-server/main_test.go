package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fora/internal/db"
)

func TestRunImportSeedsDefaultBoards(t *testing.T) {
	tempDir := t.TempDir()
	exportPath := filepath.Join(tempDir, "export.json")
	dbPath := filepath.Join(tempDir, "fora.db")

	if err := os.WriteFile(exportPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write export file: %v", err)
	}

	if err := runImport([]string{"--from", exportPath, "--db", dbPath}); err != nil {
		t.Fatalf("runImport: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	boards, err := db.ListBoards(context.Background(), database)
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
		t.Fatalf("unexpected board ids after import startup path: got %v want %v", ids, want)
	}
}
