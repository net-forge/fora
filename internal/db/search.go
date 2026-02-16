package db

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"fora/internal/models"
)

type SearchParams struct {
	Query       string
	Author      string
	Tag         string
	Board       string
	Since       *time.Time
	ThreadsOnly bool
	Limit       int
	Offset      int
}

func searchWhereClause(params SearchParams) (string, []any) {
	whereClause := " WHERE content_fts MATCH ?"
	args := []any{params.Query}

	if strings.TrimSpace(params.Author) != "" {
		whereClause += " AND c.author = ?"
		args = append(args, strings.TrimSpace(params.Author))
	}
	if strings.TrimSpace(params.Tag) != "" {
		whereClause += " AND EXISTS (SELECT 1 FROM tags t WHERE t.content_id = c.id AND t.tag = ?)"
		args = append(args, strings.TrimSpace(params.Tag))
	}
	if strings.TrimSpace(params.Board) != "" {
		whereClause += " AND c.board_id = ?"
		args = append(args, strings.TrimSpace(params.Board))
	}
	if params.Since != nil {
		whereClause += " AND c.created >= ?"
		args = append(args, params.Since.UTC().Format(time.RFC3339))
	}
	if params.ThreadsOnly {
		whereClause += " AND c.type = 'post'"
	}
	return whereClause, args
}

func CountSearchContent(ctx context.Context, database *sql.DB, params SearchParams) (int, error) {
	whereClause, args := searchWhereClause(params)
	query := `
SELECT COUNT(*)
FROM content_fts
JOIN content c ON c.rowid = content_fts.rowid` + whereClause

	var total int
	if err := database.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
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

	whereClause, args := searchWhereClause(params)
	query := `
SELECT c.id, c.type, COALESCE(c.title, ''), c.author, c.thread_id, COALESCE(c.board_id, ''), c.created,
       snippet(content_fts, 2, '>>>', '<<<', '...', 20) AS snippet
FROM content_fts
JOIN content c ON c.rowid = content_fts.rowid` + whereClause +
		" ORDER BY c.created DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.SearchResult, 0)
	for rows.Next() {
		var r models.SearchResult
		if err := rows.Scan(&r.ID, &r.Type, &r.Title, &r.Author, &r.ThreadID, &r.BoardID, &r.Created, &r.Snippet); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
