CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_active_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_last_active_idx
    ON sessions(last_active_at DESC, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('complete', 'streaming', 'cancelled', 'failed')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS messages_session_created_idx
    ON messages(session_id, created_at, id);

CREATE TABLE IF NOT EXISTS drafts (
    session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
