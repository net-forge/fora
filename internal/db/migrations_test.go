package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigrationsIdempotentAndLatestVersionApplied(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "migrations.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	if err := ApplyMigrations(database); err != nil {
		t.Fatalf("first migration apply: %v", err)
	}
	if err := ApplyMigrations(database); err != nil {
		t.Fatalf("second migration apply: %v", err)
	}

	var latest int
	if err := database.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&latest); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if latest < 5 {
		t.Fatalf("expected latest schema version >=5, got %d", latest)
	}
}
