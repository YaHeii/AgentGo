package contract

import (
	"errors"
	"time"
)

var ErrDocumentNotFound = errors.New("rag: document not found")

type Document struct {
	ID             int64
	SourcePath     string
	NormalizedPath string
	FileHash       string
	UpdatedAt      time.Time
}

type Chunk struct {
	ID          int64
	DocumentID  int64
	ChunkIndex  int64
	Content     string
	Embedding   []byte
}

type ChunkMatch struct {
	Chunk    Chunk
	Document Document
	Distance float64
}

type UpsertDocumentParams struct {
	SourcePath     string
	NormalizedPath string
	FileHash       string
	UpdatedAt      time.Time
}

type CreateChunkParams struct {
	DocumentID int64
	ChunkIndex int64
	Content    string
	Embedding  []byte
}

type SearchChunksParams struct {
	NormalizedPathGlob string
	QueryEmbedding     []byte
	TopK               int64
}
