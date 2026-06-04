-- name: CreateTask :one
INSERT INTO tasks (
    subagent_session_id,
    parent_session_id,
    kind,
    status,
    input_payload_json,
    progress_payload_json,
    result_payload_json,
    error_message,
    created_at,
    completed_at
)
VALUES (?, ?, ?, 'running', ?, ?, ?, ?, ?, NULL)
RETURNING subagent_session_id, parent_session_id, kind, status, input_payload_json, progress_payload_json, result_payload_json, error_message, created_at, completed_at;

-- name: GetTask :one
SELECT subagent_session_id, parent_session_id, kind, status, input_payload_json, progress_payload_json, result_payload_json, error_message, created_at, completed_at
FROM tasks
WHERE subagent_session_id = ?;

-- name: ListTasksByParentSession :many
SELECT subagent_session_id, parent_session_id, kind, status, input_payload_json, progress_payload_json, result_payload_json, error_message, created_at, completed_at
FROM tasks
WHERE parent_session_id = ?
ORDER BY created_at ASC, subagent_session_id ASC;

-- name: UpdateTaskProgress :one
UPDATE tasks
SET progress_payload_json = ?
WHERE subagent_session_id = ?
RETURNING subagent_session_id, parent_session_id, kind, status, input_payload_json, progress_payload_json, result_payload_json, error_message, created_at, completed_at;

-- name: CompleteTask :one
UPDATE tasks
SET status = 'complete',
    result_payload_json = ?,
    completed_at = ?
WHERE subagent_session_id = ?
RETURNING subagent_session_id, parent_session_id, kind, status, input_payload_json, progress_payload_json, result_payload_json, error_message, created_at, completed_at;

-- name: FailTask :one
UPDATE tasks
SET status = 'failed',
    error_message = ?,
    completed_at = ?
WHERE subagent_session_id = ?
RETURNING subagent_session_id, parent_session_id, kind, status, input_payload_json, progress_payload_json, result_payload_json, error_message, created_at, completed_at;
