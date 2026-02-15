package db

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"hive/internal/models"
)

type SearchParams struct {
	Query       string
	Author      string
	Tag         string
	Since       *time.Time
	ThreadsOnly bool
	Limit       int
	Offset      int
}

func SearchContent(ctx context.Context, database *sql.DB, params SearchParams) ([]models.SearchResult, error) {
	limit := params.Limit
	offset := params.Offset
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	query := `
SELECT c.id, c.type, COALESCE(c.title, ''), c.author, c.thread_id, c.created,
       snippet(content_fts, 2, '>>>', '<<<', '...', 20) AS snippet
FROM content_fts
JOIN content c ON c.rowid = content_fts.rowid
WHERE content_fts MATCH ?`
	args := []any{params.Query}

	if strings.TrimSpace(params.Author) != "" {
		query += " AND c.author = ?"
		args = append(args, strings.TrimSpace(params.Author))
	}
	if strings.TrimSpace(params.Tag) != "" {
		query += " AND EXISTS (SELECT 1 FROM tags t WHERE t.content_id = c.id AND t.tag = ?)"
		args = append(args, strings.TrimSpace(params.Tag))
	}
	if params.Since != nil {
		query += " AND c.created >= ?"
		args = append(args, params.Since.UTC().Format(time.RFC3339))
	}
	if params.ThreadsOnly {
		query += " AND c.type = 'post'"
	}
	query += " ORDER BY c.created DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.SearchResult, 0)
	for rows.Next() {
		var r models.SearchResult
		if err := rows.Scan(&r.ID, &r.Type, &r.Title, &r.Author, &r.ThreadID, &r.Created, &r.Snippet); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
