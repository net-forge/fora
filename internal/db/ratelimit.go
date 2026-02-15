package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CountContentByAuthorSince returns the number of content rows authored since the given time.
// If contentType is empty, both posts and replies are counted.
func CountContentByAuthorSince(ctx context.Context, database *sql.DB, author string, since time.Time, contentType string) (int, *time.Time, error) {
	query := `
SELECT COUNT(1), MIN(created)
FROM content
WHERE author = ? AND created >= ?`
	args := []any{author, since.UTC().Format(time.RFC3339)}

	if strings.TrimSpace(contentType) != "" {
		query += " AND type = ?"
		args = append(args, contentType)
	}

	var (
		count       int
		oldestValue sql.NullString
	)
	if err := database.QueryRowContext(ctx, query, args...).Scan(&count, &oldestValue); err != nil {
		return 0, nil, err
	}

	if !oldestValue.Valid || strings.TrimSpace(oldestValue.String) == "" {
		return count, nil, nil
	}
	oldest, err := time.Parse(time.RFC3339, oldestValue.String)
	if err != nil {
		return 0, nil, fmt.Errorf("parse oldest created timestamp: %w", err)
	}
	oldest = oldest.UTC()
	return count, &oldest, nil
}
