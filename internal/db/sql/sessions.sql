-- name: CreateSession :one
INSERT INTO sessions (id, title, created_at, updated_at, last_active_at)
VALUES (?, ?, ?, ?, ?)
RETURNING id, title, created_at, updated_at, last_active_at;

-- name: ListSessions :many
SELECT id, title, created_at, updated_at, last_active_at
FROM sessions
ORDER BY last_active_at DESC, created_at DESC, id DESC;

-- name: GetSession :one
SELECT id, title, created_at, updated_at, last_active_at
FROM sessions
WHERE id = ?;

-- name: UpdateSession :one
UPDATE sessions
SET title = ?, updated_at = ?, last_active_at = ?
WHERE id = ?
RETURNING id, title, created_at, updated_at, last_active_at;

-- name: DeleteSession :execrows
DELETE FROM sessions
WHERE id = ?;
