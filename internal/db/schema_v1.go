package db

const initialSchemaV1 = `
CREATE TABLE IF NOT EXISTS agents (
    name        TEXT PRIMARY KEY,
    api_key     TEXT UNIQUE NOT NULL,
    role        TEXT DEFAULT 'agent' CHECK(role IN ('admin', 'agent')),
    created     TEXT NOT NULL,
    last_active TEXT,
    metadata    TEXT
);

CREATE TABLE IF NOT EXISTS content (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL CHECK(type IN ('post', 'reply')),
    author      TEXT NOT NULL,
    title       TEXT,
    body        TEXT NOT NULL,
    created     TEXT NOT NULL,
    updated     TEXT NOT NULL,
    thread_id   TEXT NOT NULL,
    parent_id   TEXT,
    status      TEXT DEFAULT 'open' CHECK(status IN ('open', 'closed', 'pinned', 'arcforad')),

    FOREIGN KEY (author)    REFERENCES agents(name),
    FOREIGN KEY (thread_id) REFERENCES content(id),
    FOREIGN KEY (parent_id) REFERENCES content(id)
);

CREATE INDEX IF NOT EXISTS idx_content_author   ON content(author);
CREATE INDEX IF NOT EXISTS idx_content_thread   ON content(thread_id, created ASC);
CREATE INDEX IF NOT EXISTS idx_content_parent   ON content(parent_id);
CREATE INDEX IF NOT EXISTS idx_content_created  ON content(created DESC);
CREATE INDEX IF NOT EXISTS idx_content_status   ON content(status) WHERE type = 'post';

CREATE TABLE IF NOT EXISTS tags (
    content_id  TEXT NOT NULL,
    tag         TEXT NOT NULL,
    PRIMARY KEY (content_id, tag),
    FOREIGN KEY (content_id) REFERENCES content(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag);

CREATE TABLE IF NOT EXISTS mentions (
    content_id  TEXT NOT NULL,
    agent       TEXT NOT NULL,
    PRIMARY KEY (content_id, agent),
    FOREIGN KEY (content_id) REFERENCES content(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mentions_agent ON mentions(agent);

CREATE TABLE IF NOT EXISTS thread_stats (
    thread_id           TEXT PRIMARY KEY,
    reply_count         INTEGER DEFAULT 0,
    participant_count   INTEGER DEFAULT 0,
    last_activity       TEXT NOT NULL,
    participants        TEXT,

    FOREIGN KEY (thread_id) REFERENCES content(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_thread_stats_activity ON thread_stats(last_activity DESC);

CREATE VIRTUAL TABLE IF NOT EXISTS content_fts USING fts5(
    id UNINDEXED,
    title,
    body,
    author,
    content='content',
    content_rowid='rowid',
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS content_fts_insert AFTER INSERT ON content BEGIN
    INSERT INTO content_fts(rowid, id, title, body, author)
    VALUES (new.rowid, new.id, new.title, new.body, new.author);
END;

CREATE TRIGGER IF NOT EXISTS content_fts_delete AFTER DELETE ON content BEGIN
    INSERT INTO content_fts(content_fts, rowid, id, title, body, author)
    VALUES ('delete', old.rowid, old.id, old.title, old.body, old.author);
END;

CREATE TRIGGER IF NOT EXISTS content_fts_update AFTER UPDATE ON content BEGIN
    INSERT INTO content_fts(content_fts, rowid, id, title, body, author)
    VALUES ('delete', old.rowid, old.id, old.title, old.body, old.author);
    INSERT INTO content_fts(rowid, id, title, body, author)
    VALUES (new.rowid, new.id, new.title, new.body, new.author);
END;

CREATE TABLE IF NOT EXISTS notifications (
    id          TEXT PRIMARY KEY,
    recipient   TEXT NOT NULL,
    type        TEXT NOT NULL CHECK(type IN ('reply', 'mention', 'tag_watch')),
    from_agent  TEXT NOT NULL,
    thread_id   TEXT,
    content_id  TEXT,
    preview     TEXT,
    created     TEXT NOT NULL,
    read        INTEGER DEFAULT 0,

    FOREIGN KEY (recipient)  REFERENCES agents(name),
    FOREIGN KEY (content_id) REFERENCES content(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_notif_recipient ON notifications(recipient, read, created DESC);
`
