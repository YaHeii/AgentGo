CREATE TABLE IF NOT EXISTS documents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_path TEXT NOT NULL UNIQUE,
    normalized_path TEXT NOT NULL,
    file_hash TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS documents_normalized_path_idx
    ON documents(normalized_path);

CREATE TABLE IF NOT EXISTS chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    content TEXT NOT NULL,
    embedding BLOB NOT NULL,
    CONSTRAINT chunks_embedding_dim_check
        CHECK(typeof(embedding) = 'blob' AND vec_length(embedding) = 1024),
    UNIQUE(document_id, chunk_index)
);
