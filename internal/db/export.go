package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
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
		byID := make(map[string]models.Content, len(items))
		replyPaths := make(map[string]string, len(items))
		for _, item := range items {
			byID[item.ID] = item
		}

		var replyPath func(replyID string, seen map[string]bool) string
		replyPath = func(replyID string, seen map[string]bool) string {
			if path, ok := replyPaths[replyID]; ok {
				return path
			}
			item, ok := byID[replyID]
			if !ok {
				return filepath.ToSlash(filepath.Join("threads", threadID, "replies", replyID, "reply.md"))
			}
			if seen[replyID] {
				return filepath.ToSlash(filepath.Join("threads", threadID, "replies", replyID, "reply.md"))
			}
			parentID := ""
			if item.ParentID != nil {
				parentID = strings.TrimSpace(*item.ParentID)
			}
			if parentID == "" || parentID == threadID {
				path := filepath.ToSlash(filepath.Join("threads", threadID, "replies", replyID, "reply.md"))
				replyPaths[replyID] = path
				return path
			}

			parent, ok := byID[parentID]
			if !ok || parent.Type != "reply" {
				path := filepath.ToSlash(filepath.Join("threads", threadID, "replies", replyID, "reply.md"))
				replyPaths[replyID] = path
				return path
			}

			nextSeen := make(map[string]bool, len(seen)+1)
			for id, v := range seen {
				nextSeen[id] = v
			}
			nextSeen[replyID] = true
			parentPath := replyPath(parentID, nextSeen)
			path := filepath.ToSlash(filepath.Join(filepath.Dir(parentPath), "replies", replyID, "reply.md"))
			replyPaths[replyID] = path
			return path
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
				Path:    replyPath(item.ID, map[string]bool{}),
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
	meta := struct {
		ID       string   `yaml:"id"`
		Type     string   `yaml:"type"`
		Author   string   `yaml:"author"`
		Title    *string  `yaml:"title,omitempty"`
		Created  string   `yaml:"created"`
		Updated  string   `yaml:"updated"`
		ThreadID string   `yaml:"thread_id"`
		ParentID *string  `yaml:"parent_id,omitempty"`
		Status   string   `yaml:"status"`
		Tags     []string `yaml:"tags,omitempty"`
	}{
		ID:       c.ID,
		Type:     c.Type,
		Author:   c.Author,
		Title:    c.Title,
		Created:  c.Created,
		Updated:  c.Updated,
		ThreadID: c.ThreadID,
		ParentID: c.ParentID,
		Status:   c.Status,
		Tags:     c.Tags,
	}
	b, _ := yaml.Marshal(meta)
	return fmt.Sprintf("---\n%s---", string(b))
}
