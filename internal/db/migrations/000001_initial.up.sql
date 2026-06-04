CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    message_count INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    cost_micros INTEGER NOT NULL DEFAULT 0,
    summary_message_id TEXT NOT NULL DEFAULT '',
    todos_json TEXT NOT NULL DEFAULT '[]',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_last_active_idx
    ON sessions(updated_at DESC, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    provider TEXT NOT NULL,
    finished_at INTEGER NOT NULL,
    is_compact_summary INTEGER NOT NULL DEFAULT 0,
    message_json TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS messages_session_created_idx
    ON messages(session_id, finished_at, id);

CREATE TABLE IF NOT EXISTS tasks (
    subagent_session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    parent_session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'complete', 'failed')),
    input_payload_json TEXT,
    progress_payload_json TEXT,
    result_payload_json TEXT,
    error_message TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    completed_at INTEGER
);

CREATE INDEX IF NOT EXISTS tasks_parent_session_status_idx
    ON tasks(parent_session_id, status);

CREATE INDEX IF NOT EXISTS tasks_kind_status_idx
    ON tasks(kind, status);
