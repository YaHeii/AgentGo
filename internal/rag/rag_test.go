package rag

import (
	"context"
	"testing"
	"time"

	db "github.com/YaHeii/agentGo/internal/db"
	ragcontract "github.com/YaHeii/agentGo/internal/rag/contract"
)

var _ ragStore = (*db.Store)(nil)
var _ ragTxStore = (db.TxStore)(nil)

func TestRagStoreAndTxStoreInterfacesSupportSearchAndTransactionalIndexing(t *testing.T) {
	store := &fakeRagStore{
		document: ragcontract.Document{
			ID:             7,
			SourcePath:     "docs/guide.md",
			NormalizedPath: "docs/guide.md",
			FileHash:       "hash-1",
			UpdatedAt:      time.UnixMilli(1700000000000).UTC(),
		},
		matches: []ragcontract.ChunkMatch{
			{
				Chunk: ragcontract.Chunk{
					ID:         3,
					DocumentID: 7,
					ChunkIndex: 0,
					Content:    "retrieved chunk",
				},
			},
		},
	}
	tx := &fakeRagTxStore{}

	matches, err := exerciseRagStore(context.Background(), store, tx)
	if err != nil {
		t.Fatalf("exerciseRagStore returned error: %v", err)
	}
	if len(matches) != 1 || matches[0].Chunk.Content != "retrieved chunk" {
		t.Fatalf("unexpected matches: %#v", matches)
	}
	if !store.getDocumentCalled {
		t.Fatal("expected GetDocumentBySourcePath to be used")
	}
	if !store.searchCalled {
		t.Fatal("expected SearchChunksByPrefix to be used")
	}
	if !tx.upsertCalled {
		t.Fatal("expected UpsertDocument to be used inside tx")
	}
	if !tx.deleteChunksCalled {
		t.Fatal("expected DeleteChunksByDocumentID to be used inside tx")
	}
	if !tx.createChunkCalled {
		t.Fatal("expected CreateChunk to be used inside tx")
	}
	if !tx.deleteDocumentCalled {
		t.Fatal("expected DeleteDocumentBySourcePath to be supported inside tx")
	}
}

func exerciseRagStore(ctx context.Context, store ragStore, tx ragTxStore) ([]ragcontract.ChunkMatch, error) {
	_, err := store.GetDocumentBySourcePath(ctx, "docs/guide.md")
	if err != nil {
		return nil, err
	}

	doc, err := tx.UpsertDocument(ctx, ragcontract.UpsertDocumentParams{
		SourcePath:     "docs/guide.md",
		NormalizedPath: "docs/guide.md",
		FileHash:       "hash-2",
		UpdatedAt:      time.UnixMilli(1700000001000).UTC(),
	})
	if err != nil {
		return nil, err
	}
	if err := tx.DeleteChunksByDocumentID(ctx, doc.ID); err != nil {
		return nil, err
	}
	if _, err := tx.CreateChunk(ctx, ragcontract.CreateChunkParams{
		DocumentID: doc.ID,
		ChunkIndex: 0,
		Content:    "chunk body",
		Embedding:  []byte("embedding"),
	}); err != nil {
		return nil, err
	}
	if err := tx.DeleteDocumentBySourcePath(ctx, "docs/obsolete.md"); err != nil {
		return nil, err
	}

	return store.SearchChunksByPrefix(ctx, ragcontract.SearchChunksParams{
		NormalizedPathGlob: "docs/*",
		QueryEmbedding:     []byte("query"),
		TopK:               3,
	})
}

type fakeRagStore struct {
	document          ragcontract.Document
	matches           []ragcontract.ChunkMatch
	getDocumentCalled bool
	searchCalled      bool
}

func (s *fakeRagStore) GetDocumentBySourcePath(ctx context.Context, sourcePath string) (ragcontract.Document, error) {
	s.getDocumentCalled = true
	return s.document, nil
}

func (s *fakeRagStore) SearchChunksByPrefix(ctx context.Context, params ragcontract.SearchChunksParams) ([]ragcontract.ChunkMatch, error) {
	s.searchCalled = true
	return s.matches, nil
}

type fakeRagTxStore struct {
	upsertCalled         bool
	deleteDocumentCalled bool
	deleteChunksCalled   bool
	createChunkCalled    bool
}

func (s *fakeRagTxStore) UpsertDocument(ctx context.Context, params ragcontract.UpsertDocumentParams) (ragcontract.Document, error) {
	s.upsertCalled = true
	return ragcontract.Document{
		ID:             7,
		SourcePath:     params.SourcePath,
		NormalizedPath: params.NormalizedPath,
		FileHash:       params.FileHash,
		UpdatedAt:      params.UpdatedAt,
	}, nil
}

func (s *fakeRagTxStore) DeleteDocumentBySourcePath(ctx context.Context, sourcePath string) error {
	s.deleteDocumentCalled = true
	return nil
}

func (s *fakeRagTxStore) DeleteChunksByDocumentID(ctx context.Context, documentID int64) error {
	s.deleteChunksCalled = true
	return nil
}

func (s *fakeRagTxStore) CreateChunk(ctx context.Context, params ragcontract.CreateChunkParams) (ragcontract.Chunk, error) {
	s.createChunkCalled = true
	return ragcontract.Chunk{
		ID:         1,
		DocumentID: params.DocumentID,
		ChunkIndex: params.ChunkIndex,
		Content:    params.Content,
		Embedding:  append([]byte(nil), params.Embedding...),
	}, nil
}
