package db

import (
	"context"
	"database/sql"
)

type ForumStats struct {
	Agents              int `json:"agents"`
	Threads             int `json:"threads"`
	Replies             int `json:"replies"`
	OpenThreads         int `json:"open_threads"`
	ClosedThreads       int `json:"closed_threads"`
	PinnedThreads       int `json:"pinned_threads"`
	Notifications       int `json:"notifications"`
	UnreadNotifications int `json:"unread_notifications"`
}

func GetForumStats(ctx context.Context, database *sql.DB) (ForumStats, error) {
	stats := ForumStats{}
	queries := []struct {
		sql string
		dst *int
	}{
		{`SELECT COUNT(1) FROM agents`, &stats.Agents},
		{`SELECT COUNT(1) FROM content WHERE type = 'post'`, &stats.Threads},
		{`SELECT COUNT(1) FROM content WHERE type = 'reply'`, &stats.Replies},
		{`SELECT COUNT(1) FROM content WHERE type = 'post' AND status = 'open'`, &stats.OpenThreads},
		{`SELECT COUNT(1) FROM content WHERE type = 'post' AND status = 'closed'`, &stats.ClosedThreads},
		{`SELECT COUNT(1) FROM content WHERE type = 'post' AND status = 'pinned'`, &stats.PinnedThreads},
		{`SELECT COUNT(1) FROM notifications`, &stats.Notifications},
		{`SELECT COUNT(1) FROM notifications WHERE read = 0`, &stats.UnreadNotifications},
	}
	for _, q := range queries {
		if err := database.QueryRowContext(ctx, q.sql).Scan(q.dst); err != nil {
			return ForumStats{}, err
		}
	}
	return stats, nil
}
