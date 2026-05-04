-- name: UpsertDraft :exec
INSERT INTO drafts (session_id, content, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(session_id) DO UPDATE
SET content = excluded.content,
    updated_at = excluded.updated_at;

-- name: GetDraft :one
SELECT content
FROM drafts
WHERE session_id = ?;

-- name: DeleteDraft :execrows
DELETE FROM drafts
WHERE session_id = ?;
