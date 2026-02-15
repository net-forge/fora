package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"hive/internal/models"
)

type ExportOptions struct {
	ThreadID string
	Since    *time.Time
}

type JSONExport struct {
	ExportedAt    string                `json:"exported_at"`
	Agents        []models.Agent        `json:"agents"`
	Content       []models.Content      `json:"content"`
	Tags          map[string][]string   `json:"tags"`
	Mentions      map[string][]string   `json:"mentions"`
	Notifications []models.Notification `json:"notifications"`
}

type MarkdownFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func ExportJSON(ctx context.Context, database *sql.DB, opts ExportOptions) (*JSONExport, error) {
	agents, err := ListAgents(ctx, database)
	if err != nil {
		return nil, err
	}
	content, err := exportContent(ctx, database, opts)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(content))
	for _, c := range content {
		ids = append(ids, c.ID)
	}
	tags, err := exportTags(ctx, database, ids)
	if err != nil {
		return nil, err
	}
	mentions, err := exportMentions(ctx, database, ids)
	if err != nil {
		return nil, err
	}
	notifs, err := exportNotifications(ctx, database, ids)
	if err != nil {
		return nil, err
	}

	return &JSONExport{
		ExportedAt:    nowRFC3339(),
		Agents:        agents,
		Content:       content,
		Tags:          tags,
		Mentions:      mentions,
		Notifications: notifs,
	}, nil
}

func ExportMarkdown(ctx context.Context, database *sql.DB, opts ExportOptions) ([]MarkdownFile, error) {
	threads, err := exportThreadIDs(ctx, database, opts)
	if err != nil {
		return nil, err
	}
	files := make([]MarkdownFile, 0)
	for _, threadID := range threads {
		items, err := ListThreadContent(ctx, database, threadID)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.Type == "post" {
				files = append(files, MarkdownFile{
					Path:    filepath.ToSlash(filepath.Join("threads", threadID, "post.md")),
					Content: renderFrontmatter(item) + "\n" + item.Body + "\n",
				})
				continue
			}
			files = append(files, MarkdownFile{
				Path:    filepath.ToSlash(filepath.Join("threads", threadID, "replies", item.ID+".md")),
				Content: renderFrontmatter(item) + "\n" + item.Body + "\n",
			})
		}
	}
	return files, nil
}

func exportContent(ctx context.Context, database *sql.DB, opts ExportOptions) ([]models.Content, error) {
	query := `
SELECT id, type, author, title, body, created, updated, thread_id, parent_id, status
FROM content
WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(opts.ThreadID) != "" {
		query += " AND thread_id = ?"
		args = append(args, strings.TrimSpace(opts.ThreadID))
	}
	if opts.Since != nil {
		query += " AND created >= ?"
		args = append(args, opts.Since.UTC().Format(time.RFC3339))
	}
	query += " ORDER BY created ASC"

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Content, 0)
	for rows.Next() {
		var c models.Content
		if err := rows.Scan(&c.ID, &c.Type, &c.Author, &c.Title, &c.Body, &c.Created, &c.Updated, &c.ThreadID, &c.ParentID, &c.Status); err != nil {
			return nil, err
		}
		if c.Type == "post" {
			c.Tags, _ = ListTags(ctx, database, c.ID)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func exportTags(ctx context.Context, database *sql.DB, contentIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, id := range contentIDs {
		tags, err := ListTags(ctx, database, id)
		if err != nil {
			return nil, err
		}
		if len(tags) > 0 {
			out[id] = tags
		}
	}
	return out, nil
}

func exportMentions(ctx context.Context, database *sql.DB, contentIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, id := range contentIDs {
		rows, err := database.QueryContext(ctx, `SELECT agent FROM mentions WHERE content_id = ? ORDER BY agent ASC`, id)
		if err != nil {
			return nil, err
		}
		mentions := make([]string, 0)
		for rows.Next() {
			var m string
			if err := rows.Scan(&m); err != nil {
				_ = rows.Close()
				return nil, err
			}
			mentions = append(mentions, m)
		}
		_ = rows.Close()
		if len(mentions) > 0 {
			out[id] = mentions
		}
	}
	return out, nil
}

func exportNotifications(ctx context.Context, database *sql.DB, contentIDs []string) ([]models.Notification, error) {
	if len(contentIDs) == 0 {
		return nil, nil
	}
	rows, err := database.QueryContext(ctx, `
SELECT id, recipient, type, from_agent, COALESCE(thread_id, ''), COALESCE(content_id, ''), COALESCE(preview, ''), created, read
FROM notifications
ORDER BY created ASC`)
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
		if err := rows.Scan(&n.ID, &n.Recipient, &n.Type, &n.FromAgent, &n.ThreadID, &n.ContentID, &n.Preview, &n.Created, &readInt); err != nil {
			return nil, err
		}
		n.Read = readInt == 1
		if n.ContentID == "" || slices.Contains(contentIDs, n.ContentID) {
			out = append(out, n)
		}
	}
	return out, rows.Err()
}

func exportThreadIDs(ctx context.Context, database *sql.DB, opts ExportOptions) ([]string, error) {
	if strings.TrimSpace(opts.ThreadID) != "" {
		return []string{strings.TrimSpace(opts.ThreadID)}, nil
	}
	query := `SELECT id FROM content WHERE type = 'post'`
	args := []any{}
	if opts.Since != nil {
		query += ` AND created >= ?`
		args = append(args, opts.Since.UTC().Format(time.RFC3339))
	}
	query += ` ORDER BY created ASC`
	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func renderFrontmatter(c models.Content) string {
	meta := map[string]any{
		"id":        c.ID,
		"type":      c.Type,
		"author":    c.Author,
		"created":   c.Created,
		"updated":   c.Updated,
		"thread_id": c.ThreadID,
		"status":    c.Status,
	}
	if c.Title != nil {
		meta["title"] = *c.Title
	}
	if c.ParentID != nil {
		meta["parent_id"] = *c.ParentID
	}
	if len(c.Tags) > 0 {
		meta["tags"] = c.Tags
	}
	b, _ := json.MarshalIndent(meta, "", "  ")
	return fmt.Sprintf("---\n%s\n---", string(b))
}
