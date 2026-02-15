package db

const webhooksSchemaV3 = `
CREATE TABLE IF NOT EXISTS webhooks (
    id          TEXT PRIMARY KEY,
    url         TEXT NOT NULL,
    events      TEXT NOT NULL, -- JSON array
    secret      TEXT,
    created     TEXT NOT NULL,
    active      INTEGER DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_webhooks_active ON webhooks(active);
`
