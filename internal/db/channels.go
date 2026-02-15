package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type Channel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Created     string `json:"created"`
}

func CreateChannel(ctx context.Context, database *sql.DB, name, description string) (*Channel, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	id := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	created := time.Now().UTC().Format(time.RFC3339)
	if _, err := database.ExecContext(ctx, `
INSERT INTO channels (id, name, description, created)
VALUES (?, ?, ?, ?)`, id, name, nullableString(description), created); err != nil {
		return nil, err
	}
	return &Channel{ID: id, Name: name, Description: description, Created: created}, nil
}

func ListChannels(ctx context.Context, database *sql.DB) ([]Channel, error) {
	rows, err := database.QueryContext(ctx, `
SELECT id, name, COALESCE(description, ''), created
FROM channels
ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Channel, 0)
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.Created); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func ChannelExists(ctx context.Context, database *sql.DB, channelID string) (bool, error) {
	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(1) FROM channels WHERE id = ?`, channelID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
