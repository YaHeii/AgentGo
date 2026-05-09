-- name: CreateMessage :one
INSERT INTO messages (id, session_id, kind, provider, finished_at, is_compact_summary, message_json)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id, session_id, kind, provider, finished_at, is_compact_summary, message_json;

-- name: ListMessages :many
SELECT id, session_id, kind, provider, finished_at, is_compact_summary, message_json
FROM messages
WHERE session_id = ?
ORDER BY finished_at ASC, id ASC;
