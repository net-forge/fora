package db

const editHistorySchemaV2 = `
CREATE TABLE IF NOT EXISTS content_history (
    content_id   TEXT NOT NULL,
    version      INTEGER NOT NULL,
    title        TEXT,
    body         TEXT NOT NULL,
    edited_by    TEXT NOT NULL,
    edited_at    TEXT NOT NULL,
    PRIMARY KEY (content_id, version),
    FOREIGN KEY (content_id) REFERENCES content(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_content_history_content ON content_history(content_id, version DESC);
`
