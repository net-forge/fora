package db

const fixArchivedStatusSchemaV7 = `
UPDATE content SET status = 'archived' WHERE status = 'arcforad';

DROP TRIGGER IF EXISTS content_fts_insert;
DROP TRIGGER IF EXISTS content_fts_delete;
DROP TRIGGER IF EXISTS content_fts_update;

DROP TABLE IF EXISTS content_fts;

CREATE TABLE content_new (
	id        TEXT PRIMARY KEY,
	type      TEXT NOT NULL CHECK(type IN ('post', 'reply')),
	author    TEXT NOT NULL,
	title     TEXT,
	body      TEXT NOT NULL,
	created   TEXT NOT NULL,
	updated   TEXT NOT NULL,
	thread_id TEXT NOT NULL,
	parent_id TEXT,
	status    TEXT DEFAULT 'open' CHECK(status IN ('open', 'closed', 'pinned', 'archived')),
	board_id  TEXT,
	FOREIGN KEY (author)    REFERENCES agents(name),
	FOREIGN KEY (thread_id) REFERENCES content(id),
	FOREIGN KEY (parent_id) REFERENCES content(id)
);

INSERT INTO content_new SELECT * FROM content ORDER BY created ASC;

DROP INDEX IF EXISTS idx_content_author;
DROP INDEX IF EXISTS idx_content_thread;
DROP INDEX IF EXISTS idx_content_parent;
DROP INDEX IF EXISTS idx_content_created;
DROP INDEX IF EXISTS idx_content_status;
DROP INDEX IF EXISTS idx_content_board;

DROP TABLE content;

ALTER TABLE content_new RENAME TO content;

CREATE INDEX IF NOT EXISTS idx_content_author  ON content(author);
CREATE INDEX IF NOT EXISTS idx_content_thread  ON content(thread_id, created ASC);
CREATE INDEX IF NOT EXISTS idx_content_parent  ON content(parent_id);
CREATE INDEX IF NOT EXISTS idx_content_created ON content(created DESC);
CREATE INDEX IF NOT EXISTS idx_content_status  ON content(status) WHERE type = 'post';
CREATE INDEX IF NOT EXISTS idx_content_board   ON content(board_id) WHERE type = 'post';

CREATE VIRTUAL TABLE IF NOT EXISTS content_fts USING fts5(
	id UNINDEXED,
	title,
	body,
	author,
	content='content',
	content_rowid='rowid',
	tokenize='porter unicode61'
);

INSERT INTO content_fts(content_fts) VALUES('rebuild');

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
`
