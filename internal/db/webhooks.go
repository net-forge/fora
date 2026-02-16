package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"fora/internal/models"
)

func CreateWebhook(ctx context.Context, database *sql.DB, url string, events []string, secret string) (*models.Webhook, error) {
	if len(events) == 0 {
		return nil, errors.New("at least one event is required")
	}
	eventsJSON, err := json.Marshal(dedupeTags(events))
	if err != nil {
		return nil, err
	}
	id, err := generateWebhookID()
	if err != nil {
		return nil, err
	}
	created := nowRFC3339()
	if _, err := database.ExecContext(ctx, `
INSERT INTO webhooks (id, url, events, secret, created, active)
VALUES (?, ?, ?, ?, ?, 1)`,
		id, url, string(eventsJSON), nullableString(secret), created); err != nil {
		return nil, err
	}
	return &models.Webhook{
		ID:      id,
		URL:     url,
		Events:  dedupeTags(events),
		Secret:  secret,
		Created: created,
		Active:  true,
	}, nil
}

func ListWebhooks(ctx context.Context, database *sql.DB, activeOnly bool) ([]models.Webhook, error) {
	query := `SELECT id, url, events, COALESCE(secret, ''), created, active FROM webhooks`
	if activeOnly {
		query += ` WHERE active = 1`
	}
	query += ` ORDER BY created ASC`
	rows, err := database.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Webhook, 0)
	for rows.Next() {
		var (
			w         models.Webhook
			eventsRaw string
			activeInt int
		)
		if err := rows.Scan(&w.ID, &w.URL, &eventsRaw, &w.Secret, &w.Created, &activeInt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(eventsRaw), &w.Events)
		w.Active = activeInt == 1
		out = append(out, w)
	}
	return out, rows.Err()
}

func DeleteWebhook(ctx context.Context, database *sql.DB, id string) error {
	res, err := database.ExecContext(ctx, `DELETE FROM webhooks WHERE id = ?`, id)
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

func generateWebhookID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "wh_" + hex.EncodeToString(b), nil
}
