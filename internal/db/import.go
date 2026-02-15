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

	"gopkg.in/yaml.v3"
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

	type markdownRecord struct {
		content models.Content
		tags    []string
	}
	records := make([]markdownRecord, 0, len(files))

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		meta, body, err := parseFrontmatter(string(content))
		if err != nil {
			return fmt.Errorf("parse %s: %w", f, err)
		}
		id := toString(meta["id"])
		typ := toString(meta["type"])
		author := toString(meta["author"])
		created := toString(meta["created"])
		updated := toString(meta["updated"])
		threadID := toString(meta["thread_id"])
		status := toString(meta["status"])
		title := toStringPtr(meta["title"])
		parentID := toStringPtr(meta["parent_id"])
		if id == "" || typ == "" || author == "" || created == "" || updated == "" || threadID == "" {
			return fmt.Errorf("incomplete frontmatter in %s", f)
		}
		if status == "" {
			status = "open"
		}
		records = append(records, markdownRecord{
			content: models.Content{
				ID:       id,
				Type:     typ,
				Author:   author,
				Title:    title,
				Body:     strings.TrimSpace(body),
				Created:  created,
				Updated:  updated,
				ThreadID: threadID,
				ParentID: parentID,
				Status:   status,
			},
			tags: parseTags(meta["tags"]),
		})
	}

	insertRecord := func(record markdownRecord) error {
		if err := ensureAgentForImportTx(ctx, tx, record.content.Author); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO content (id, type, author, title, body, created, updated, thread_id, parent_id, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			record.content.ID, record.content.Type, record.content.Author, record.content.Title,
			record.content.Body, record.content.Created, record.content.Updated, record.content.ThreadID,
			record.content.ParentID, record.content.Status); err != nil {
			return err
		}

		for _, tag := range record.tags {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags (content_id, tag) VALUES (?, ?)`, record.content.ID, tag); err != nil {
				return err
			}
		}
		return nil
	}

	pendingReplies := make([]markdownRecord, 0)
	for _, record := range records {
		if record.content.Type == "post" {
			if err := insertRecord(record); err != nil {
				return err
			}
			continue
		}
		pendingReplies = append(pendingReplies, record)
	}

	for len(pendingReplies) > 0 {
		nextPending := make([]markdownRecord, 0, len(pendingReplies))
		progressed := false
		for _, record := range pendingReplies {
			parentID := ""
			if record.content.ParentID != nil {
				parentID = strings.TrimSpace(*record.content.ParentID)
			}
			if parentID != "" {
				var parentExists int
				if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM content WHERE id = ?`, parentID).Scan(&parentExists); err != nil {
					return err
				}
				if parentExists == 0 {
					nextPending = append(nextPending, record)
					continue
				}
			}
			if err := insertRecord(record); err != nil {
				return err
			}
			progressed = true
		}
		if len(nextPending) == 0 {
			break
		}
		if !progressed {
			return fmt.Errorf("could not resolve parent references for %d markdown replies", len(nextPending))
		}
		pendingReplies = nextPending
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
	normalized := strings.ReplaceAll(strings.TrimPrefix(in, "\ufeff"), "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return nil, "", errors.New("missing frontmatter")
	}
	rest := normalized[len("---\n"):]
	sepIndex := strings.Index(rest, "\n---\n")
	sepLen := len("\n---\n")
	if sepIndex == -1 {
		sepIndex = strings.Index(rest, "\n---")
		sepLen = len("\n---")
	}
	if sepIndex == -1 {
		return nil, "", errors.New("missing closing frontmatter delimiter")
	}
	frontmatterBlock := strings.TrimSpace(rest[:sepIndex])
	var meta map[string]any
	if err := yaml.Unmarshal([]byte(frontmatterBlock), &meta); err != nil {
		return nil, "", err
	}
	body := strings.TrimSpace(rest[sepIndex+sepLen:])
	return meta, body, nil
}

func toStringPtr(v any) *string {
	s := toString(v)
	if s == "" {
		return nil
	}
	return &s
}

func toString(v any) string {
	switch raw := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(raw)
	case time.Time:
		return raw.UTC().Format(time.RFC3339)
	default:
		return strings.TrimSpace(fmt.Sprint(raw))
	}
}

func parseTags(raw any) []string {
	switch tags := raw.(type) {
	case []any:
		out := make([]string, 0, len(tags))
		for _, t := range tags {
			tag := toString(t)
			if tag != "" {
				out = append(out, tag)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(tags))
		for _, t := range tags {
			tag := strings.TrimSpace(t)
			if tag != "" {
				out = append(out, tag)
			}
		}
		return out
	default:
		return nil
	}
}
