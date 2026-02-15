package db

import (
	"context"
	"database/sql"
	"strings"

	"hive/internal/models"
)

func ListNotifications(ctx context.Context, database *sql.DB, recipient string, includeRead bool, limit, offset int) ([]models.Notification, error) {
	query := `
SELECT id, recipient, type, from_agent, COALESCE(thread_id, ''), COALESCE(content_id, ''), COALESCE(preview, ''), created, read
FROM notifications
WHERE recipient = ?`
	args := []any{recipient}
	if !includeRead {
		query += " AND read = 0"
	}
	query += " ORDER BY created DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Notification, 0)
	for rows.Next() {
		var (
			n       models.Notification
			readInt int
		)
		if err := rows.Scan(
			&n.ID, &n.Recipient, &n.Type, &n.FromAgent,
			&n.ThreadID, &n.ContentID, &n.Preview, &n.Created, &readInt,
		); err != nil {
			return nil, err
		}
		n.Read = readInt == 1
		out = append(out, n)
	}
	return out, rows.Err()
}

func MarkNotificationRead(ctx context.Context, database *sql.DB, recipient, id string) error {
	res, err := database.ExecContext(ctx, `
UPDATE notifications
SET read = 1
WHERE id = ? AND recipient = ?`, id, recipient)
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

func MarkAllNotificationsRead(ctx context.Context, database *sql.DB, recipient string) (int64, error) {
	res, err := database.ExecContext(ctx, `
UPDATE notifications
SET read = 1
WHERE recipient = ? AND read = 0`, recipient)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func createNotificationTx(
	ctx context.Context,
	tx *sql.Tx,
	recipient, notifType, fromAgent, threadID, contentID, preview string,
) error {
	preview = strings.TrimSpace(preview)
	if len(preview) > 200 {
		preview = preview[:200]
	}
	_, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO notifications (id, recipient, type, from_agent, thread_id, content_id, preview, created, read)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		generateNotificationID(recipient+notifType+contentID),
		recipient, notifType, fromAgent, nullableString(threadID), nullableString(contentID), nullableString(preview),
		nowRFC3339(),
	)
	return err
}

func nullableString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
