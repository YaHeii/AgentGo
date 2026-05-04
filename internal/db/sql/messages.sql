-- name: CreateMessage :one
INSERT INTO messages (id, session_id, role, content, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING id, session_id, role, content, status, created_at, updated_at;

-- name: ListMessages :many
SELECT id, session_id, role, content, status, created_at, updated_at
FROM messages
WHERE session_id = ?
ORDER BY created_at ASC, id ASC;

-- name: UpdateMessage :one
UPDATE messages
SET content = ?, status = ?, updated_at = ?
WHERE id = ?
RETURNING id, session_id, role, content, status, created_at, updated_at;
