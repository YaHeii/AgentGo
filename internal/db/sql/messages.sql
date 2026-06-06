-- name: CreateMessage :one
INSERT INTO messages (id, session_id, kind, provider, finished_at, is_compact_summary, message_json)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id, session_id, kind, provider, finished_at, is_compact_summary, message_json;

-- name: ListMessages :many
SELECT id, session_id, kind, provider, finished_at, is_compact_summary, message_json
FROM messages
WHERE session_id = ?
ORDER BY finished_at ASC, id ASC;

-- name: GetMessage :one
SELECT id, session_id, kind, provider, finished_at, is_compact_summary, message_json
FROM messages
WHERE id = ?;

-- name: DeleteMessage :execrows
DELETE FROM messages
WHERE id = ?;

-- name: DeleteSessionMessages :execrows
DELETE FROM messages
WHERE session_id = ?;
