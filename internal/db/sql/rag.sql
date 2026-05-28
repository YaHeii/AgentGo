-- name: UpsertDocument :one
INSERT INTO documents (
    source_path,
    normalized_path,
    file_hash,
    updated_at
)
VALUES (?, ?, ?, ?)
ON CONFLICT(source_path) DO UPDATE
SET normalized_path = excluded.normalized_path,
    file_hash = excluded.file_hash,
    updated_at = excluded.updated_at
RETURNING id, source_path, normalized_path, file_hash, updated_at;

-- name: GetDocumentBySourcePath :one
SELECT id, source_path, normalized_path, file_hash, updated_at
FROM documents
WHERE source_path = ?;

-- name: DeleteDocumentBySourcePath :execrows
DELETE FROM documents
WHERE source_path = ?;

-- name: CreateChunk :one
INSERT INTO chunks (
    document_id,
    chunk_index,
    content,
    embedding
)
VALUES (?, ?, ?, ?)
RETURNING id, document_id, chunk_index, content, embedding;

-- name: ListChunksByDocumentID :many
SELECT id, document_id, chunk_index, content, embedding
FROM chunks
WHERE document_id = ?
ORDER BY chunk_index ASC, id ASC;

-- name: DeleteChunksByDocumentID :execrows
DELETE FROM chunks
WHERE document_id = ?;

-- name: SearchChunksByPrefix :many
SELECT
    c.id,
    c.document_id,
    c.chunk_index,
    c.content,
    c.embedding,
    d.id AS document_id_alias,
    d.source_path,
    d.normalized_path,
    d.file_hash,
    d.updated_at,
    vec_distance_cosine(c.embedding, sqlc.arg(query_embedding)) AS distance
FROM chunks AS c
JOIN documents AS d
    ON d.id = c.document_id
WHERE d.normalized_path GLOB sqlc.arg(normalized_path_glob)
ORDER BY distance ASC, c.id ASC
LIMIT sqlc.arg(top_k);
