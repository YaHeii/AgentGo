package rag

import (
	"context"

	ragcontract "github.com/YaHeii/agentGo/internal/rag/contract"
)

type ragStore interface {
	GetDocumentBySourcePath(ctx context.Context, sourcePath string) (ragcontract.Document, error)
	SearchChunksByPrefix(ctx context.Context, params ragcontract.SearchChunksParams) ([]ragcontract.ChunkMatch, error)
}

type ragTxStore interface {
	UpsertDocument(ctx context.Context, params ragcontract.UpsertDocumentParams) (ragcontract.Document, error)
	DeleteDocumentBySourcePath(ctx context.Context, sourcePath string) error
	DeleteChunksByDocumentID(ctx context.Context, documentID int64) error
	CreateChunk(ctx context.Context, params ragcontract.CreateChunkParams) (ragcontract.Chunk, error)
}
