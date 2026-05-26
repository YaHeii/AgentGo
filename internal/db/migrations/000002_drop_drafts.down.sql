CREATE TABLE IF NOT EXISTS drafts (
    session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
