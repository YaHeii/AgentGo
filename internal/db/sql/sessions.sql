-- name: CreateSession :one
INSERT INTO sessions (
    id,
    title,
    message_count,
    completion_tokens,
    cost_micros,
    summary_message_id,
    todos_json,
    created_at,
    updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, title, message_count, completion_tokens, cost_micros, summary_message_id, todos_json, created_at, updated_at;

-- name: ListSessions :many
SELECT id, title, message_count, completion_tokens, cost_micros, summary_message_id, todos_json, created_at, updated_at
FROM sessions
ORDER BY updated_at DESC, created_at DESC, id DESC;

-- name: GetSession :one
SELECT id, title, message_count, completion_tokens, cost_micros, summary_message_id, todos_json, created_at, updated_at
FROM sessions
WHERE id = ?;

-- name: UpdateSession :one
UPDATE sessions
SET title = ?,
    message_count = ?,
    completion_tokens = ?,
    cost_micros = ?,
    summary_message_id = ?,
    todos_json = ?,
    updated_at = ?
WHERE id = ?
RETURNING id, title, message_count, completion_tokens, cost_micros, summary_message_id, todos_json, created_at, updated_at;

-- name: DeleteSession :execrows
DELETE FROM sessions
WHERE id = ?;
