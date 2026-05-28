package rag

import (
	"context"
	"testing"

	ragcontract "github.com/YaHeii/agentGo/internal/rag/contract"
)

func TestNewServiceKeepsStoreAndConfig(t *testing.T) {
	store := &stubServiceRagStore{}
	cfg := Config{
		APIKey:             "rag-key",
		EmbeddingBaseURL:   "https://rag.example.com/v1/embeddings",
		EmbeddingDimension: 1024,
		EmbeddingModel:     "BAAI/bge-large-zh-v1.5",
		RerankBaseURL:      "https://rag.example.com/v1/rerank",
		RerankModel:        "BAAI/bge-reranker-v2-m3",
	}

	svc := NewService(store, cfg)
	if svc == nil {
		t.Fatal("expected non-nil rag service")
	}
	if svc.store != store {
		t.Fatal("expected service to keep rag store")
	}
	if svc.cfg != cfg {
		t.Fatalf("expected config %#v, got %#v", cfg, svc.cfg)
	}
}

type stubServiceRagStore struct{}

func (s *stubServiceRagStore) GetDocumentBySourcePath(_ context.Context, _ string) (ragcontract.Document, error) {
	return ragcontract.Document{}, nil
}

func (s *stubServiceRagStore) SearchChunksByPrefix(_ context.Context, _ ragcontract.SearchChunksParams) ([]ragcontract.ChunkMatch, error) {
	return nil, nil
}
