package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type Board struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Created     string   `json:"created"`
}

func CreateBoard(ctx context.Context, database *sql.DB, name, description, icon string, tags []string) (*Board, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	id := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	description = strings.TrimSpace(description)
	icon = strings.TrimSpace(icon)
	created := time.Now().UTC().Format(time.RFC3339)

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO boards (id, name, description, icon, created)
VALUES (?, ?, ?, ?, ?)`, id, name, nullableString(description), nullableString(icon), created); err != nil {
		return nil, err
	}
	if err := upsertBoardTagsTx(ctx, tx, id, tags); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &Board{ID: id, Name: name, Description: description, Icon: icon, Tags: dedupeTags(tags), Created: created}, nil
}

func ListBoards(ctx context.Context, database *sql.DB) ([]Board, error) {
	rows, err := database.QueryContext(ctx, `
SELECT id, name, COALESCE(description, ''), COALESCE(icon, ''), created
FROM boards
ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Board, 0)
	for rows.Next() {
		var b Board
		if err := rows.Scan(&b.ID, &b.Name, &b.Description, &b.Icon, &b.Created); err != nil {
			return nil, err
		}
		tags, err := ListBoardTags(ctx, database, b.ID)
		if err != nil {
			return nil, err
		}
		b.Tags = tags
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func BoardExists(ctx context.Context, database *sql.DB, boardID string) (bool, error) {
	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(1) FROM boards WHERE id = ?`, boardID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func GetBoard(ctx context.Context, database *sql.DB, id string) (*Board, error) {
	row := database.QueryRowContext(ctx, `
SELECT id, name, COALESCE(description, ''), COALESCE(icon, ''), created
FROM boards
WHERE id = ?`, strings.TrimSpace(id))

	b := &Board{}
	if err := row.Scan(&b.ID, &b.Name, &b.Description, &b.Icon, &b.Created); err != nil {
		return nil, err
	}
	tags, err := ListBoardTags(ctx, database, b.ID)
	if err != nil {
		return nil, err
	}
	b.Tags = tags
	return b, nil
}

func UpdateBoardTags(ctx context.Context, database *sql.DB, boardID string, add, remove []string) ([]string, error) {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	exists, err := boardExistsTx(ctx, tx, boardID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, sql.ErrNoRows
	}

	if err := upsertBoardTagsTx(ctx, tx, boardID, add); err != nil {
		return nil, err
	}
	remove = dedupeTags(remove)
	if len(remove) > 0 {
		placeholders := make([]string, 0, len(remove))
		args := make([]any, 0, len(remove)+1)
		args = append(args, boardID)
		for _, tag := range remove {
			placeholders = append(placeholders, "?")
			args = append(args, tag)
		}
		query := `DELETE FROM board_tags WHERE board_id = ? AND tag IN (` + strings.Join(placeholders, ", ") + `)`
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return nil, err
		}
	}

	tags, err := listBoardTagsTx(ctx, tx, boardID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tags, nil
}

func SubscribeToBoard(ctx context.Context, database *sql.DB, boardID, agent string) error {
	_, err := database.ExecContext(ctx, `
INSERT OR IGNORE INTO board_subscriptions (board_id, agent, created)
VALUES (?, ?, ?)`, boardID, agent, nowRFC3339())
	return err
}

func UnsubscribeFromBoard(ctx context.Context, database *sql.DB, boardID, agent string) error {
	_, err := database.ExecContext(ctx, `
DELETE FROM board_subscriptions
WHERE board_id = ? AND agent = ?`, boardID, agent)
	return err
}

func ListBoardSubscribers(ctx context.Context, database *sql.DB, boardID string) ([]string, error) {
	rows, err := database.QueryContext(ctx, `
SELECT agent
FROM board_subscriptions
WHERE board_id = ?
ORDER BY agent ASC`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var agent string
		if err := rows.Scan(&agent); err != nil {
			return nil, err
		}
		out = append(out, agent)
	}
	return out, rows.Err()
}

func ListAgentSubscriptions(ctx context.Context, database *sql.DB, agent string) ([]string, error) {
	rows, err := database.QueryContext(ctx, `
SELECT board_id
FROM board_subscriptions
WHERE agent = ?
ORDER BY board_id ASC`, agent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var boardID string
		if err := rows.Scan(&boardID); err != nil {
			return nil, err
		}
		out = append(out, boardID)
	}
	return out, rows.Err()
}

func ListBoardTags(ctx context.Context, database *sql.DB, boardID string) ([]string, error) {
	rows, err := database.QueryContext(ctx, `
SELECT tag FROM board_tags
WHERE board_id = ?
ORDER BY tag ASC`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func listBoardTagsTx(ctx context.Context, tx *sql.Tx, boardID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT tag FROM board_tags
WHERE board_id = ?
ORDER BY tag ASC`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func upsertBoardTagsTx(ctx context.Context, tx *sql.Tx, boardID string, tags []string) error {
	for _, tag := range dedupeTags(tags) {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO board_tags (board_id, tag) VALUES (?, ?)`,
			boardID, tag,
		); err != nil {
			return err
		}
	}
	return nil
}

func boardExistsTx(ctx context.Context, tx *sql.Tx, boardID string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM boards WHERE id = ?`, boardID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
