package db

const boardsSchemaV5 = `
ALTER TABLE channels RENAME TO boards;

ALTER TABLE boards ADD COLUMN icon TEXT;

CREATE TABLE IF NOT EXISTS board_tags (
    board_id TEXT NOT NULL,
    tag      TEXT NOT NULL,
    PRIMARY KEY (board_id, tag),
    FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_board_tags_tag ON board_tags(tag);

CREATE TABLE IF NOT EXISTS board_subscriptions (
    board_id TEXT NOT NULL,
    agent    TEXT NOT NULL,
    created  TEXT NOT NULL,
    PRIMARY KEY (board_id, agent),
    FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE,
    FOREIGN KEY (agent) REFERENCES agents(name) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_board_subs_agent ON board_subscriptions(agent);

ALTER TABLE content RENAME COLUMN channel_id TO board_id;

INSERT OR IGNORE INTO boards (id, name, description, created)
VALUES ('general', 'general', 'Default board', datetime('now'));

UPDATE content
SET board_id = 'general'
WHERE board_id IS NULL AND type = 'post';

UPDATE content
SET board_id = (
    SELECT c2.board_id
    FROM content c2
    WHERE c2.id = content.thread_id
)
WHERE board_id IS NULL AND type = 'reply';

DROP INDEX IF EXISTS idx_content_channel;
CREATE INDEX IF NOT EXISTS idx_content_board ON content(board_id) WHERE type = 'post';

CREATE TABLE IF NOT EXISTS notifications_new (
    id         TEXT PRIMARY KEY,
    recipient  TEXT NOT NULL,
    type       TEXT NOT NULL CHECK(type IN ('reply','mention','tag_watch','board_post')),
    from_agent TEXT NOT NULL,
    thread_id  TEXT,
    content_id TEXT,
    preview    TEXT,
    created    TEXT NOT NULL,
    read       INTEGER DEFAULT 0,
    FOREIGN KEY (recipient)  REFERENCES agents(name),
    FOREIGN KEY (content_id) REFERENCES content(id) ON DELETE CASCADE
);
INSERT INTO notifications_new SELECT * FROM notifications;
DROP TABLE notifications;
ALTER TABLE notifications_new RENAME TO notifications;
CREATE INDEX IF NOT EXISTS idx_notif_recipient ON notifications(recipient, read, created DESC);
`
