package db

import (
	"database/sql"
	"fmt"
)

type migration struct {
	version int
	name    string
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		name:    "initial_schema",
		sql:     initialSchemaV1,
	},
	{
		version: 2,
		name:    "edit_history",
		sql:     editHistorySchemaV2,
	},
	{
		version: 3,
		name:    "webhooks",
		sql:     webhooksSchemaV3,
	},
	{
		version: 4,
		name:    "channels",
		sql:     channelsSchemaV4,
	},
}

func ApplyMigrations(database *sql.DB) error {
	if _, err := database.Exec(`
CREATE TABLE IF NOT EXISTS schema_version (
	version     INTEGER PRIMARY KEY,
	name        TEXT NOT NULL,
	applied_at  TEXT NOT NULL
);`); err != nil {
		return fmt.Errorf("ensure schema_version table: %w", err)
	}

	for _, m := range migrations {
		applied, err := migrationApplied(database, m.version)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", m.version, err)
		}
		if applied {
			continue
		}
		if err := applyMigration(database, m); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
		}
	}

	return nil
}

func migrationApplied(database *sql.DB, version int) (bool, error) {
	var count int
	if err := database.QueryRow(
		"SELECT COUNT(1) FROM schema_version WHERE version = ?",
		version,
	).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func applyMigration(database *sql.DB, m migration) error {
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(m.sql); err != nil {
		return err
	}
	if _, err := tx.Exec(
		"INSERT INTO schema_version (version, name, applied_at) VALUES (?, ?, datetime('now'))",
		m.version, m.name,
	); err != nil {
		return err
	}

	return tx.Commit()
}
