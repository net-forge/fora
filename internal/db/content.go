package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"hive/internal/models"
)

type CreateContentParams struct {
	Type     string
	Author   string
	Title    *string
	Body     string
	ThreadID string
	ParentID *string
	Status   string
	Tags     []string
}

type ListPostsParams struct {
	Limit  int
	Offset int

	Author  string
	Tag     string
	Channel string
	Status  string
	Since   *time.Time
	Sort    string
	Order   string
}

type ListActivityParams struct {
	Limit  int
	Offset int
	Author string
}

func CreatePost(ctx context.Context, database *sql.DB, author string, title *string, body string, tags []string, mentions []string, channelID *string) (*models.Content, error) {
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("body is required")
	}
	id := generateContentID(body)
	now := nowRFC3339()
	status := "open"

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
INSERT INTO content (id, type, author, title, body, created, updated, thread_id, parent_id, status, channel_id)
VALUES (?, 'post', ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
		id, author, title, body, now, now, id, status, channelID)
	if err != nil {
		if isUniqueConstraint(err) {
			existing, getErr := getContentTx(ctx, tx, id)
			if getErr != nil {
				return nil, getErr
			}
			existing.Tags, _ = listTagsTx(ctx, tx, existing.ID)
			return existing, nil
		}
		return nil, err
	}

	if err := upsertTagsTx(ctx, tx, id, tags); err != nil {
		return nil, err
	}
	resolvedMentions := normalizeMentions(mentions, body)
	if err := upsertMentionsTx(ctx, tx, id, resolvedMentions); err != nil {
		return nil, err
	}
	if err := createMentionNotificationsTx(ctx, tx, author, id, id, body, resolvedMentions); err != nil {
		return nil, err
	}
	if err := insertInitialThreadStatsTx(ctx, tx, id, author, now); err != nil {
		return nil, err
	}

	c := &models.Content{
		ID:        id,
		Type:      "post",
		Author:    author,
		Title:     title,
		Body:      body,
		Created:   now,
		Updated:   now,
		ThreadID:  id,
		ParentID:  nil,
		Status:    status,
		ChannelID: channelID,
		Tags:      dedupeTags(tags),
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return c, nil
}

func CreateReply(ctx context.Context, database *sql.DB, author string, parentID string, body string, mentions []string) (*models.Content, error) {
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("body is required")
	}
	parent, err := GetContent(ctx, database, parentID)
	if err != nil {
		return nil, err
	}

	id := generateContentID(body)
	now := nowRFC3339()
	status := "open"
	threadID := parent.ThreadID

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
INSERT INTO content (id, type, author, title, body, created, updated, thread_id, parent_id, status)
VALUES (?, 'reply', ?, NULL, ?, ?, ?, ?, ?, ?)`,
		id, author, body, now, now, threadID, parentID, status)
	if err != nil {
		if isUniqueConstraint(err) {
			existing, getErr := getContentTx(ctx, tx, id)
			if getErr != nil {
				return nil, getErr
			}
			return existing, nil
		}
		return nil, err
	}
	if err := upsertThreadStatsForReplyTx(ctx, tx, threadID, author, now); err != nil {
		return nil, err
	}
	resolvedMentions := normalizeMentions(mentions, body)
	if err := upsertMentionsTx(ctx, tx, id, resolvedMentions); err != nil {
		return nil, err
	}
	if err := createMentionNotificationsTx(ctx, tx, author, threadID, id, body, resolvedMentions); err != nil {
		return nil, err
	}
	if err := createReplyNotificationsTx(ctx, tx, author, parent, id, body); err != nil {
		return nil, err
	}

	c := &models.Content{
		ID:       id,
		Type:     "reply",
		Author:   author,
		Title:    nil,
		Body:     body,
		Created:  now,
		Updated:  now,
		ThreadID: threadID,
		ParentID: &parentID,
		Status:   status,
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return c, nil
}

func GetContent(ctx context.Context, database *sql.DB, id string) (*models.Content, error) {
	row := database.QueryRowContext(ctx, `
SELECT id, type, author, title, body, created, updated, thread_id, parent_id, status, channel_id
FROM content
WHERE id = ?`, id)
	c := &models.Content{}
	if err := row.Scan(
		&c.ID, &c.Type, &c.Author, &c.Title, &c.Body, &c.Created,
		&c.Updated, &c.ThreadID, &c.ParentID, &c.Status, &c.ChannelID,
	); err != nil {
		return nil, err
	}
	if c.Type == "post" {
		c.Tags, _ = ListTags(ctx, database, c.ID)
	}
	return c, nil
}

func ListPosts(ctx context.Context, database *sql.DB, params ListPostsParams) ([]models.ThreadListItem, int, error) {
	limit := params.Limit
	offset := params.Offset
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	sortField := "ts.last_activity"
	switch params.Sort {
	case "", "activity":
		sortField = "ts.last_activity"
	case "created":
		sortField = "c.created"
	case "replies":
		sortField = "COALESCE(ts.reply_count, 0)"
	}
	order := "DESC"
	if strings.EqualFold(params.Order, "asc") {
		order = "ASC"
	}

	whereClause, whereArgs := listPostsWhereClause(params)

	countQuery := `
SELECT COUNT(*)
FROM content c
LEFT JOIN thread_stats ts ON ts.thread_id = c.id` + whereClause
	var total int
	if err := database.QueryRowContext(ctx, countQuery, whereArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
SELECT c.id, c.type, c.author, c.title, c.body, c.created, c.updated, c.thread_id, c.parent_id, c.status, c.channel_id,
       COALESCE(ts.reply_count, 0), COALESCE(ts.last_activity, c.created), COALESCE(ts.participants, '[]'), COALESCE(ts.participant_count, 1)
FROM content c
LEFT JOIN thread_stats ts ON ts.thread_id = c.id` + whereClause +
		" ORDER BY " + sortField + " " + order + ", c.created DESC LIMIT ? OFFSET ?"

	args := append(append([]any{}, whereArgs...), limit, offset)
	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]models.ThreadListItem, 0)
	for rows.Next() {
		var (
			item             models.ThreadListItem
			participantsJSON string
		)
		if err := rows.Scan(
			&item.ID, &item.Type, &item.Author, &item.Title, &item.Body, &item.Created,
			&item.Updated, &item.ThreadID, &item.ParentID, &item.Status, &item.ChannelID,
			&item.ReplyCount, &item.LastActivity, &participantsJSON, &item.ParticipantCount,
		); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal([]byte(participantsJSON), &item.Participants)
		item.Tags, _ = ListTags(ctx, database, item.ID)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func listPostsWhereClause(params ListPostsParams) (string, []any) {
	var args []any
	whereClause := "\nWHERE c.type = 'post'"
	if params.Author != "" {
		whereClause += " AND c.author = ?"
		args = append(args, params.Author)
	}
	if params.Status != "" {
		whereClause += " AND c.status = ?"
		args = append(args, params.Status)
	}
	if params.Tag != "" {
		whereClause += " AND EXISTS (SELECT 1 FROM tags t WHERE t.content_id = c.id AND t.tag = ?)"
		args = append(args, params.Tag)
	}
	if params.Channel != "" {
		whereClause += " AND c.channel_id = ?"
		args = append(args, params.Channel)
	}
	if params.Since != nil {
		whereClause += " AND COALESCE(ts.last_activity, c.created) >= ?"
		args = append(args, params.Since.UTC().Format(time.RFC3339))
	}
	return whereClause, args
}

func ListReplies(ctx context.Context, database *sql.DB, parentID string, limit, offset int) ([]models.Content, error) {
	rows, err := database.QueryContext(ctx, `
SELECT id, type, author, title, body, created, updated, thread_id, parent_id, status, channel_id
FROM content
WHERE parent_id = ? AND type = 'reply'
ORDER BY created ASC
LIMIT ? OFFSET ?`, parentID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Content, 0)
	for rows.Next() {
		var c models.Content
		if err := rows.Scan(
			&c.ID, &c.Type, &c.Author, &c.Title, &c.Body, &c.Created,
			&c.Updated, &c.ThreadID, &c.ParentID, &c.Status, &c.ChannelID,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func ListActivity(ctx context.Context, database *sql.DB, params ListActivityParams) ([]models.ActivityEvent, error) {
	limit := params.Limit
	offset := params.Offset
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var args []any
	query := `
SELECT c.id, c.type, c.author, c.title, c.thread_id, c.parent_id, c.created
FROM content c
WHERE c.type IN ('post', 'reply')`

	if params.Author != "" {
		query += " AND c.author = ?"
		args = append(args, params.Author)
	}

	query += " ORDER BY c.created DESC, c.rowid DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.ActivityEvent, 0)
	for rows.Next() {
		var item models.ActivityEvent
		if err := rows.Scan(
			&item.ID, &item.Type, &item.Author, &item.Title, &item.ThreadID, &item.ParentID, &item.Created,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func ResolveThreadID(ctx context.Context, database *sql.DB, id string) (string, error) {
	var threadID string
	if err := database.QueryRowContext(ctx, `
SELECT thread_id
FROM content
WHERE id = ?`, id).Scan(&threadID); err != nil {
		return "", err
	}
	return threadID, nil
}

func ListThreadContent(ctx context.Context, database *sql.DB, threadID string) ([]models.Content, error) {
	rows, err := database.QueryContext(ctx, `
SELECT id, type, author, title, body, created, updated, thread_id, parent_id, status, channel_id
FROM content
WHERE thread_id = ?
ORDER BY created ASC`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Content, 0)
	for rows.Next() {
		var c models.Content
		if err := rows.Scan(
			&c.ID, &c.Type, &c.Author, &c.Title, &c.Body, &c.Created,
			&c.Updated, &c.ThreadID, &c.ParentID, &c.Status, &c.ChannelID,
		); err != nil {
			return nil, err
		}
		if c.Type == "post" {
			c.Tags, _ = ListTags(ctx, database, c.ID)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func UpdatePost(ctx context.Context, database *sql.DB, id string, title *string, body string, editedBy string) (*models.Content, error) {
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("body is required")
	}
	now := nowRFC3339()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var (
		oldTitle *string
		oldBody  string
	)
	if err := tx.QueryRowContext(ctx, `
SELECT title, body
FROM content
WHERE id = ? AND type = 'post'`, id).Scan(&oldTitle, &oldBody); err != nil {
		return nil, err
	}

	var version int
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(version), 0)
FROM content_history
WHERE content_id = ?`, id).Scan(&version); err != nil {
		return nil, err
	}
	version++

	if _, err := tx.ExecContext(ctx, `
INSERT INTO content_history (content_id, version, title, body, edited_by, edited_at)
VALUES (?, ?, ?, ?, ?, ?)`, id, version, oldTitle, oldBody, editedBy, now); err != nil {
		return nil, err
	}

	res, err := tx.ExecContext(ctx, `
UPDATE content
SET title = ?, body = ?, updated = ?
WHERE id = ? AND type = 'post'`, title, body, now, id)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return GetContent(ctx, database, id)
}

func UpdateReply(ctx context.Context, database *sql.DB, id string, body string, editedBy string) (*models.Content, error) {
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("body is required")
	}
	now := nowRFC3339()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var (
		oldTitle *string
		oldBody  string
	)
	if err := tx.QueryRowContext(ctx, `
SELECT title, body
FROM content
WHERE id = ? AND type = 'reply'`, id).Scan(&oldTitle, &oldBody); err != nil {
		return nil, err
	}

	var version int
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(version), 0)
FROM content_history
WHERE content_id = ?`, id).Scan(&version); err != nil {
		return nil, err
	}
	version++

	if _, err := tx.ExecContext(ctx, `
INSERT INTO content_history (content_id, version, title, body, edited_by, edited_at)
VALUES (?, ?, ?, ?, ?, ?)`, id, version, oldTitle, oldBody, editedBy, now); err != nil {
		return nil, err
	}

	res, err := tx.ExecContext(ctx, `
UPDATE content
SET body = ?, updated = ?
WHERE id = ? AND type = 'reply'`, body, now, id)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return GetContent(ctx, database, id)
}

func ListContentHistory(ctx context.Context, database *sql.DB, contentID string) ([]models.ContentHistory, error) {
	rows, err := database.QueryContext(ctx, `
SELECT content_id, version, title, body, edited_by, edited_at
FROM content_history
WHERE content_id = ?
ORDER BY version DESC`, contentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.ContentHistory, 0)
	for rows.Next() {
		var h models.ContentHistory
		if err := rows.Scan(&h.ContentID, &h.Version, &h.Title, &h.Body, &h.EditedBy, &h.EditedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func UpdatePostStatus(ctx context.Context, database *sql.DB, id string, status string) (*models.Content, error) {
	switch status {
	case "open", "closed", "pinned", "archived":
	default:
		return nil, errors.New("invalid status")
	}
	res, err := database.ExecContext(ctx, `
UPDATE content
SET status = ?, updated = ?
WHERE id = ? AND type = 'post'`, status, nowRFC3339(), id)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, sql.ErrNoRows
	}
	return GetContent(ctx, database, id)
}

func DeletePostThread(ctx context.Context, database *sql.DB, id string) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var t string
	if err := tx.QueryRowContext(ctx, `SELECT type FROM content WHERE id = ?`, id).Scan(&t); err != nil {
		return err
	}
	if t != "post" {
		return errors.New("target is not a post")
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE content_id IN (SELECT id FROM content WHERE thread_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM mentions WHERE content_id IN (SELECT id FROM content WHERE thread_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM notifications WHERE content_id IN (SELECT id FROM content WHERE thread_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM content WHERE thread_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM thread_stats WHERE thread_id = ?`, id); err != nil {
		return err
	}

	return tx.Commit()
}

func DeleteReply(ctx context.Context, database *sql.DB, id string) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var (
		contentType string
		threadID    string
	)
	if err := tx.QueryRowContext(ctx, `
SELECT type, thread_id
FROM content
WHERE id = ?`, id).Scan(&contentType, &threadID); err != nil {
		return err
	}
	if contentType != "reply" {
		return errors.New("target is not a reply")
	}

	if _, err := tx.ExecContext(ctx, `
WITH RECURSIVE subtree(id) AS (
	SELECT id FROM content WHERE id = ?
	UNION ALL
	SELECT c.id
	FROM content c
	INNER JOIN subtree s ON c.parent_id = s.id
)
DELETE FROM content
WHERE id IN (SELECT id FROM subtree)`, id); err != nil {
		return err
	}

	if err := rebuildThreadStatsTx(ctx, tx, threadID); err != nil {
		return err
	}

	return tx.Commit()
}

func ListTags(ctx context.Context, database *sql.DB, contentID string) ([]string, error) {
	rows, err := database.QueryContext(ctx, `
SELECT tag FROM tags
WHERE content_id = ?
ORDER BY tag ASC`, contentID)
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

func UpdatePostTags(ctx context.Context, database *sql.DB, postID string, addTags, removeTags []string) ([]string, error) {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var contentType string
	if err := tx.QueryRowContext(ctx, `SELECT type FROM content WHERE id = ?`, postID).Scan(&contentType); err != nil {
		return nil, err
	}
	if contentType != "post" {
		return nil, errors.New("target is not a post")
	}

	if err := upsertTagsTx(ctx, tx, postID, addTags); err != nil {
		return nil, err
	}
	removeTags = dedupeTags(removeTags)
	if len(removeTags) > 0 {
		placeholders := make([]string, 0, len(removeTags))
		args := make([]any, 0, len(removeTags)+1)
		args = append(args, postID)
		for _, t := range removeTags {
			placeholders = append(placeholders, "?")
			args = append(args, t)
		}
		query := `DELETE FROM tags WHERE content_id = ? AND tag IN (` + strings.Join(placeholders, ", ") + `)`
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return nil, err
		}
	}
	tags, err := listTagsTx(ctx, tx, postID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tags, nil
}

func listTagsTx(ctx context.Context, tx *sql.Tx, contentID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT tag FROM tags
WHERE content_id = ?
ORDER BY tag ASC`, contentID)
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

func upsertTagsTx(ctx context.Context, tx *sql.Tx, contentID string, tags []string) error {
	for _, tag := range dedupeTags(tags) {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO tags (content_id, tag) VALUES (?, ?)`,
			contentID, tag,
		); err != nil {
			return err
		}
	}
	return nil
}

func upsertMentionsTx(ctx context.Context, tx *sql.Tx, contentID string, mentions []string) error {
	for _, mention := range mentions {
		exists, err := agentExistsTx(ctx, tx, mention)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO mentions (content_id, agent) VALUES (?, ?)`,
			contentID, mention,
		); err != nil {
			return err
		}
	}
	return nil
}

func dedupeTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

var mentionPattern = regexp.MustCompile(`@([a-zA-Z0-9][a-zA-Z0-9_-]{0,63})`)

func normalizeMentions(explicit []string, body string) []string {
	candidates := make([]string, 0, len(explicit)+4)
	candidates = append(candidates, explicit...)
	for _, match := range mentionPattern.FindAllStringSubmatch(body, -1) {
		if len(match) > 1 {
			candidates = append(candidates, match[1])
		}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func generateContentID(body string) string {
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	sum := sha256.Sum256([]byte(body))
	hashPart := hex.EncodeToString(sum[:])[:8]
	return fmt.Sprintf("%s-%s", timestamp, hashPart)
}

func getContentTx(ctx context.Context, tx *sql.Tx, id string) (*models.Content, error) {
	row := tx.QueryRowContext(ctx, `
SELECT id, type, author, title, body, created, updated, thread_id, parent_id, status, channel_id
FROM content
WHERE id = ?`, id)
	c := &models.Content{}
	if err := row.Scan(
		&c.ID, &c.Type, &c.Author, &c.Title, &c.Body, &c.Created,
		&c.Updated, &c.ThreadID, &c.ParentID, &c.Status, &c.ChannelID,
	); err != nil {
		return nil, err
	}
	return c, nil
}

func isUniqueConstraint(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique")
}

func generateNotificationID(seed string) string {
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	sum := sha256.Sum256([]byte(seed))
	hashPart := hex.EncodeToString(sum[:])[:8]
	return fmt.Sprintf("notif-%s-%s", timestamp, hashPart)
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func agentExistsTx(ctx context.Context, tx *sql.Tx, name string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM agents WHERE name = ?`, name).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func createMentionNotificationsTx(
	ctx context.Context,
	tx *sql.Tx,
	fromAgent, threadID, contentID, body string,
	mentions []string,
) error {
	for _, recipient := range mentions {
		if recipient == fromAgent {
			continue
		}
		if err := createNotificationTx(ctx, tx, recipient, "mention", fromAgent, threadID, contentID, body); err != nil {
			return err
		}
	}
	return nil
}

func createReplyNotificationsTx(
	ctx context.Context,
	tx *sql.Tx,
	fromAgent string,
	parent *models.Content,
	contentID, body string,
) error {
	if parent == nil {
		return nil
	}
	recipients := map[string]struct{}{}
	if parent.Author != fromAgent {
		recipients[parent.Author] = struct{}{}
	}

	var threadAuthor string
	if err := tx.QueryRowContext(ctx, `
SELECT author
FROM content
WHERE id = ?`, parent.ThreadID).Scan(&threadAuthor); err == nil && threadAuthor != fromAgent {
		recipients[threadAuthor] = struct{}{}
	}

	for recipient := range recipients {
		if err := createNotificationTx(ctx, tx, recipient, "reply", fromAgent, parent.ThreadID, contentID, body); err != nil {
			return err
		}
	}
	return nil
}

func insertInitialThreadStatsTx(ctx context.Context, tx *sql.Tx, threadID, author, now string) error {
	participants, err := json.Marshal([]string{author})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO thread_stats (thread_id, reply_count, participant_count, last_activity, participants)
VALUES (?, 0, 1, ?, ?)`,
		threadID, now, string(participants))
	return err
}

func upsertThreadStatsForReplyTx(ctx context.Context, tx *sql.Tx, threadID, author, now string) error {
	var (
		replyCount       int
		participantsJSON sql.NullString
	)
	err := tx.QueryRowContext(ctx, `
SELECT reply_count, participants
FROM thread_stats
WHERE thread_id = ?`, threadID).Scan(&replyCount, &participantsJSON)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	notFound := errors.Is(err, sql.ErrNoRows)

	participants := make([]string, 0)
	if participantsJSON.Valid && strings.TrimSpace(participantsJSON.String) != "" {
		_ = json.Unmarshal([]byte(participantsJSON.String), &participants)
	}
	seen := map[string]struct{}{}
	for _, p := range participants {
		seen[p] = struct{}{}
	}
	if _, ok := seen[author]; !ok {
		participants = append(participants, author)
	}
	participantsEncoded, err := json.Marshal(participants)
	if err != nil {
		return err
	}

	if notFound {
		_, err = tx.ExecContext(ctx, `
INSERT INTO thread_stats (thread_id, reply_count, participant_count, last_activity, participants)
VALUES (?, 1, ?, ?, ?)`,
			threadID, len(participants), now, string(participantsEncoded))
		return err
	}

	replyCount++
	_, err = tx.ExecContext(ctx, `
UPDATE thread_stats
SET reply_count = ?, participant_count = ?, last_activity = ?, participants = ?
WHERE thread_id = ?`,
		replyCount, len(participants), now, string(participantsEncoded), threadID)
	return err
}

func rebuildThreadStatsTx(ctx context.Context, tx *sql.Tx, threadID string) error {
	rows, err := tx.QueryContext(ctx, `
SELECT type, author, created
FROM content
WHERE thread_id = ?
ORDER BY created ASC`, threadID)
	if err != nil {
		return err
	}
	defer rows.Close()

	replyCount := 0
	lastActivity := ""
	participants := make([]string, 0)
	seen := map[string]struct{}{}

	for rows.Next() {
		var (
			contentType string
			author      string
			created     string
		)
		if err := rows.Scan(&contentType, &author, &created); err != nil {
			return err
		}
		if contentType == "reply" {
			replyCount++
		}
		lastActivity = created
		if _, ok := seen[author]; !ok {
			seen[author] = struct{}{}
			participants = append(participants, author)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if lastActivity == "" {
		_, err := tx.ExecContext(ctx, `DELETE FROM thread_stats WHERE thread_id = ?`, threadID)
		return err
	}

	participantsJSON, err := json.Marshal(participants)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO thread_stats (thread_id, reply_count, participant_count, last_activity, participants)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(thread_id) DO UPDATE SET
	reply_count = excluded.reply_count,
	participant_count = excluded.participant_count,
	last_activity = excluded.last_activity,
	participants = excluded.participants`,
		threadID, replyCount, len(participants), lastActivity, string(participantsJSON))
	return err
}
