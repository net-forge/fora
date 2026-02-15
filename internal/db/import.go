package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hive/internal/auth"
	"hive/internal/models"
)

func ImportFromPath(ctx context.Context, database *sql.DB, fromPath string) error {
	info, err := os.Stat(fromPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return ImportMarkdown(ctx, database, fromPath)
	}
	return ImportJSON(ctx, database, fromPath)
}

func ImportJSON(ctx context.Context, database *sql.DB, filePath string) error {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	var payload JSONExport
	if err := json.Unmarshal(b, &payload); err != nil {
		return fmt.Errorf("parse json export: %w", err)
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, a := range payload.Agents {
		if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO agents (name, api_key, role, created, last_active, metadata)
VALUES (?, ?, ?, ?, ?, ?)`,
			a.Name, auth.HashAPIKey("imported:"+a.Name), a.Role, a.Created, a.LastActive, a.Metadata); err != nil {
			return err
		}
	}

	insertContent := func(c models.Content) error {
		if err := ensureAgentForImportTx(ctx, tx, c.Author); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO content (id, type, author, title, body, created, updated, thread_id, parent_id, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, c.Type, c.Author, c.Title, c.Body, c.Created, c.Updated, c.ThreadID, c.ParentID, c.Status)
		return err
	}
	for _, c := range payload.Content {
		if c.Type == "post" {
			if err := insertContent(c); err != nil {
				return err
			}
		}
	}
	for _, c := range payload.Content {
		if c.Type == "reply" {
			if err := insertContent(c); err != nil {
				return err
			}
		}
	}
	for contentID, tags := range payload.Tags {
		for _, t := range tags {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags (content_id, tag) VALUES (?, ?)`, contentID, t); err != nil {
				return err
			}
		}
	}
	for contentID, mentions := range payload.Mentions {
		for _, m := range mentions {
			if err := ensureAgentForImportTx(ctx, tx, m); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO mentions (content_id, agent) VALUES (?, ?)`, contentID, m); err != nil {
				return err
			}
		}
	}
	for _, n := range payload.Notifications {
		if err := ensureAgentForImportTx(ctx, tx, n.Recipient); err != nil {
			return err
		}
		if err := ensureAgentForImportTx(ctx, tx, n.FromAgent); err != nil {
			return err
		}
		readInt := 0
		if n.Read {
			readInt = 1
		}
		if _, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO notifications (id, recipient, type, from_agent, thread_id, content_id, preview, created, read)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			n.ID, n.Recipient, n.Type, n.FromAgent, nullableString(n.ThreadID), nullableString(n.ContentID), nullableString(n.Preview), n.Created, readInt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func ImportMarkdown(ctx context.Context, database *sql.DB, dir string) error {
	files := make([]string, 0)
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return err
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		meta, body, err := parseFrontmatter(string(content))
		if err != nil {
			return fmt.Errorf("parse %s: %w", f, err)
		}
		id, _ := meta["id"].(string)
		typ, _ := meta["type"].(string)
		author, _ := meta["author"].(string)
		created, _ := meta["created"].(string)
		updated, _ := meta["updated"].(string)
		threadID, _ := meta["thread_id"].(string)
		status, _ := meta["status"].(string)
		title := toStringPtr(meta["title"])
		parentID := toStringPtr(meta["parent_id"])
		if id == "" || typ == "" || author == "" || created == "" || updated == "" || threadID == "" {
			return fmt.Errorf("incomplete frontmatter in %s", f)
		}
		if status == "" {
			status = "open"
		}
		if err := ensureAgentForImportTx(ctx, tx, author); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO content (id, type, author, title, body, created, updated, thread_id, parent_id, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, typ, author, title, strings.TrimSpace(body), created, updated, threadID, parentID, status); err != nil {
			return err
		}

		if rawTags, ok := meta["tags"]; ok {
			switch tags := rawTags.(type) {
			case []any:
				for _, t := range tags {
					tag, _ := t.(string)
					tag = strings.TrimSpace(tag)
					if tag == "" {
						continue
					}
					if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags (content_id, tag) VALUES (?, ?)`, id, tag); err != nil {
						return err
					}
				}
			}
		}
	}
	return tx.Commit()
}

func ensureAgentForImportTx(ctx context.Context, tx *sql.Tx, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("empty agent name")
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM agents WHERE name = ?`, name).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO agents (name, api_key, role, created, metadata)
VALUES (?, ?, 'agent', ?, ?)`,
		name, auth.HashAPIKey("imported:"+name), time.Now().UTC().Format(time.RFC3339), nil)
	return err
}

func parseFrontmatter(in string) (map[string]any, string, error) {
	parts := strings.SplitN(in, "---", 3)
	if len(parts) < 3 {
		return nil, "", errors.New("missing frontmatter")
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(parts[1])), &meta); err != nil {
		return nil, "", err
	}
	body := strings.TrimSpace(parts[2])
	return meta, body, nil
}

func toStringPtr(v any) *string {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}
