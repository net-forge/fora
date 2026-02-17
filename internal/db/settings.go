package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func GetSetting(ctx context.Context, database *sql.DB, key string) (string, bool, error) {
	var value string
	err := database.QueryRowContext(ctx,
		"SELECT value FROM system_settings WHERE key = ?", key,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get setting %s: %w", key, err)
	}
	return value, true, nil
}

func SetSetting(ctx context.Context, database *sql.DB, key, value string) error {
	_, err := database.ExecContext(ctx, `
		INSERT INTO system_settings (key, value, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, key, value)
	if err != nil {
		return fmt.Errorf("set setting %s: %w", key, err)
	}
	return nil
}
