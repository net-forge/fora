package db

const channelsSchemaV4 = `
CREATE TABLE IF NOT EXISTS channels (
    id          TEXT PRIMARY KEY,
    name        TEXT UNIQUE NOT NULL,
    description TEXT,
    created     TEXT NOT NULL
);

ALTER TABLE content ADD COLUMN channel_id TEXT;
CREATE INDEX IF NOT EXISTS idx_content_channel ON content(channel_id) WHERE type = 'post';
`
