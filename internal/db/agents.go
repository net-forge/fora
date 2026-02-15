package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"hive/internal/auth"
	"hive/internal/models"
)

func CreateAgent(ctx context.Context, database *sql.DB, name, role, apiKeyHash string, metadata *string) error {
	created := time.Now().UTC().Format(time.RFC3339)
	_, err := database.ExecContext(
		ctx,
		`INSERT INTO agents (name, api_key, role, created, metadata) VALUES (?, ?, ?, ?, ?)`,
		name, apiKeyHash, role, created, metadata,
	)
	return err
}

func ListAgents(ctx context.Context, database *sql.DB) ([]models.Agent, error) {
	rows, err := database.QueryContext(ctx, `
SELECT name, role, created, last_active, metadata
FROM agents
ORDER BY created ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := make([]models.Agent, 0)
	for rows.Next() {
		var a models.Agent
		if err := rows.Scan(&a.Name, &a.Role, &a.Created, &a.LastActive, &a.Metadata); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func GetAgent(ctx context.Context, database *sql.DB, name string) (*models.Agent, error) {
	var a models.Agent
	err := database.QueryRowContext(ctx, `
SELECT name, role, created, last_active, metadata
FROM agents
WHERE name = ?`, name).
		Scan(&a.Name, &a.Role, &a.Created, &a.LastActive, &a.Metadata)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func DeleteAgent(ctx context.Context, database *sql.DB, name string) error {
	res, err := database.ExecContext(ctx, `DELETE FROM agents WHERE name = ?`, name)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func CountAdmins(ctx context.Context, database *sql.DB) (int, error) {
	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(1) FROM agents WHERE role = 'admin'`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func GetAgentByAPIKeyHash(ctx context.Context, database *sql.DB, apiKeyHash string) (*models.Agent, error) {
	var a models.Agent
	err := database.QueryRowContext(ctx, `
SELECT name, role, created, last_active, metadata
FROM agents
WHERE api_key = ?`, apiKeyHash).
		Scan(&a.Name, &a.Role, &a.Created, &a.LastActive, &a.Metadata)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func EnsureBootstrapAdmin(database *sql.DB, keyOutPath string) (string, error) {
	ctx := context.Background()
	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(1) FROM agents WHERE role = 'admin'`).Scan(&count); err != nil {
		return "", fmt.Errorf("count admins: %w", err)
	}
	if count > 0 {
		return "", nil
	}

	apiKey, err := auth.GenerateAPIKey()
	if err != nil {
		return "", err
	}
	name := "admin"
	apiKeyHash := auth.HashAPIKey(apiKey)
	if err := CreateAgent(ctx, database, name, "admin", apiKeyHash, nil); err != nil {
		return "", fmt.Errorf("create bootstrap admin: %w", err)
	}

	if err := os.WriteFile(keyOutPath, []byte(apiKey+"\n"), 0o600); err != nil {
		if delErr := DeleteAgent(ctx, database, name); delErr != nil && !errors.Is(delErr, sql.ErrNoRows) {
			return "", fmt.Errorf("write key failed (%v), rollback failed (%v)", err, delErr)
		}
		return "", fmt.Errorf("write admin key file: %w", err)
	}

	return name, nil
}
