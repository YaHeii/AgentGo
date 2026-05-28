package rag

import (
	"context"
	"errors"
	"testing"

	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	ragcontract "github.com/YaHeii/agentGo/internal/rag/contract"
	"github.com/stretchr/testify/require"
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
	if svc.embeddingClient == nil {
		t.Fatal("expected service to keep concrete embedding client")
	}
	if svc.rerankClient == nil {
		t.Fatal("expected service to keep concrete rerank client")
	}
}

func TestServiceQueryRejectsInvalidParamsAndConfig(t *testing.T) {
	t.Parallel()

	store := &stubServiceRagStore{}
	cfg := Config{
		APIKey:             "rag-key",
		EmbeddingBaseURL:   "https://rag.example.com/v1/embeddings",
		EmbeddingDimension: 1024,
		EmbeddingModel:     "BAAI/bge-large-zh-v1.5",
		RerankBaseURL:      "https://rag.example.com/v1/rerank",
		RerankModel:        "BAAI/bge-reranker-v2-m3",
	}

	testCases := []struct {
		name   string
		cfg    Config
		params ragcontract.QueryParams
	}{
		{
			name: "empty query",
			cfg:  cfg,
			params: ragcontract.QueryParams{
				RawQuery:           "   ",
				NormalizedPathGlob: "docs/*",
				TopK:               2,
			},
		},
		{
			name: "empty glob",
			cfg:  cfg,
			params: ragcontract.QueryParams{
				RawQuery:           "hello",
				NormalizedPathGlob: " ",
				TopK:               2,
			},
		},
		{
			name: "invalid topk",
			cfg:  cfg,
			params: ragcontract.QueryParams{
				RawQuery:           "hello",
				NormalizedPathGlob: "docs/*",
				TopK:               0,
			},
		},
		{
			name: "missing rag config",
			cfg: Config{
				EmbeddingDimension: 1024,
			},
			params: ragcontract.QueryParams{
				RawQuery:           "hello",
				NormalizedPathGlob: "docs/*",
				TopK:               2,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newService(store, tc.cfg, newStubEmbeddingClient(), newStubRerankClient())

			msg, err := svc.Query(context.Background(), tc.params)
			require.Error(t, err)
			require.Equal(t, messagecontract.Message{}, msg)
		})
	}
}

func TestServiceQueryUsesEmbeddingRecallRerankAndAssembly(t *testing.T) {
	t.Parallel()

	store := &stubServiceRagStore{
		matches: []ragcontract.ChunkMatch{
			{
				Chunk: ragcontract.Chunk{ID: 1, ChunkIndex: 0, Content: "alpha content"},
				Document: ragcontract.Document{
					ID:         11,
					SourcePath: "docs/alpha.md",
				},
			},
			{
				Chunk: ragcontract.Chunk{ID: 2, ChunkIndex: 1, Content: "beta content"},
				Document: ragcontract.Document{
					ID:         12,
					SourcePath: "docs/beta.md",
				},
			},
			{
				Chunk: ragcontract.Chunk{ID: 3, ChunkIndex: 2, Content: "gamma content"},
				Document: ragcontract.Document{
					ID:         13,
					SourcePath: "docs/gamma.md",
				},
			},
		},
	}
	emb := &stubEmbedder{embedding: []byte("query-embedding")}
	rer := &stubReranker{
		results: []ragcontract.ChunkMatch{
			store.matches[2],
			store.matches[0],
		},
	}
	svc := newService(store, validRAGConfig(), newStubEmbeddingClientWithState(emb), newStubRerankClientWithState(rer))

	msg, err := svc.Query(context.Background(), ragcontract.QueryParams{
		RawQuery:           "find docs",
		NormalizedPathGlob: "docs/*",
		TopK:               2,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"find docs"}, emb.calls)
	require.Len(t, store.searchCalls, 1)
	require.Equal(t, ragcontract.SearchChunksParams{
		NormalizedPathGlob: "docs/*",
		QueryEmbedding:     []byte("query-embedding"),
		TopK:               10,
	}, store.searchCalls[0])
	require.Len(t, rer.calls, 1)
	require.Equal(t, "find docs", rer.calls[0].query)
	require.Equal(t, store.matches, rer.calls[0].candidates)
	require.Equal(t, int64(2), rer.calls[0].topK)

	require.Equal(t, messagecontract.KindSystem, msg.Kind)
	require.NotNil(t, msg.System)
	require.Equal(t, "rag_context", msg.System.Subtype)
	require.Len(t, msg.Parts, 1)
	require.Equal(t, messagecontract.PartTypeText, msg.Parts[0].Type)
	require.Contains(t, msg.Parts[0].Text, "Chunk 1")
	require.Contains(t, msg.Parts[0].Text, "Source: docs/gamma.md")
	require.Contains(t, msg.Parts[0].Text, "gamma content")
	require.Contains(t, msg.Parts[0].Text, "Chunk 2")
	require.Contains(t, msg.Parts[0].Text, "Source: docs/alpha.md")
	require.NotContains(t, msg.Parts[0].Text, "beta content")
}

func TestServiceQueryClampsRecallTopN(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		topK  int64
		wantK int64
	}{
		{name: "min clamp", topK: 1, wantK: 10},
		{name: "middle value", topK: 3, wantK: 12},
		{name: "max clamp", topK: 20, wantK: 50},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := &stubServiceRagStore{}
			svc := newService(store, validRAGConfig(), newStubEmbeddingClientWithState(&stubEmbedder{embedding: []byte("query")}), newStubRerankClient())

			_, err := svc.Query(context.Background(), ragcontract.QueryParams{
				RawQuery:           "find docs",
				NormalizedPathGlob: "docs/*",
				TopK:               tc.topK,
			})
			require.NoError(t, err)
			require.Len(t, store.searchCalls, 1)
			require.Equal(t, tc.wantK, store.searchCalls[0].TopK)
		})
	}
}

func TestServiceQueryReturnsEmptySystemMessageWhenNoMatches(t *testing.T) {
	t.Parallel()

	store := &stubServiceRagStore{}
	rer := &stubReranker{}
	svc := newService(store, validRAGConfig(), newStubEmbeddingClientWithState(&stubEmbedder{embedding: []byte("query")}), newStubRerankClientWithState(rer))

	msg, err := svc.Query(context.Background(), ragcontract.QueryParams{
		RawQuery:           "find docs",
		NormalizedPathGlob: "docs/*",
		TopK:               2,
	})
	require.NoError(t, err)
	require.Equal(t, messagecontract.KindSystem, msg.Kind)
	require.NotNil(t, msg.System)
	require.Len(t, msg.Parts, 1)
	require.Equal(t, "", msg.Parts[0].Text)
	require.Empty(t, rer.calls)
}

func TestServiceQueryPropagatesErrors(t *testing.T) {
	t.Parallel()

	testErr := errors.New("boom")

	t.Run("embedder", func(t *testing.T) {
		svc := newService(&stubServiceRagStore{}, validRAGConfig(), newStubEmbeddingClientWithState(&stubEmbedder{err: testErr}), newStubRerankClient())

		msg, err := svc.Query(context.Background(), ragcontract.QueryParams{
			RawQuery:           "find docs",
			NormalizedPathGlob: "docs/*",
			TopK:               2,
		})
		require.ErrorIs(t, err, testErr)
		require.Equal(t, messagecontract.Message{}, msg)
	})

	t.Run("store", func(t *testing.T) {
		svc := newService(&stubServiceRagStore{searchErr: testErr}, validRAGConfig(), newStubEmbeddingClientWithState(&stubEmbedder{embedding: []byte("query")}), newStubRerankClient())

		msg, err := svc.Query(context.Background(), ragcontract.QueryParams{
			RawQuery:           "find docs",
			NormalizedPathGlob: "docs/*",
			TopK:               2,
		})
		require.ErrorIs(t, err, testErr)
		require.Equal(t, messagecontract.Message{}, msg)
	})

	t.Run("reranker", func(t *testing.T) {
		store := &stubServiceRagStore{
			matches: []ragcontract.ChunkMatch{
				{
					Chunk: ragcontract.Chunk{Content: "alpha"},
					Document: ragcontract.Document{
						SourcePath: "docs/alpha.md",
					},
				},
			},
		}
		svc := newService(store, validRAGConfig(), newStubEmbeddingClientWithState(&stubEmbedder{embedding: []byte("query")}), newStubRerankClientWithState(&stubReranker{err: testErr}))

		msg, err := svc.Query(context.Background(), ragcontract.QueryParams{
			RawQuery:           "find docs",
			NormalizedPathGlob: "docs/*",
			TopK:               2,
		})
		require.ErrorIs(t, err, testErr)
		require.Equal(t, messagecontract.Message{}, msg)
	})
}

type stubServiceRagStore struct {
	matches     []ragcontract.ChunkMatch
	searchErr   error
	searchCalls []ragcontract.SearchChunksParams
	document    ragcontract.Document
	getDocErr   error
}

func (s *stubServiceRagStore) GetDocumentBySourcePath(_ context.Context, _ string) (ragcontract.Document, error) {
	return s.document, s.getDocErr
}

func (s *stubServiceRagStore) SearchChunksByPrefix(_ context.Context, params ragcontract.SearchChunksParams) ([]ragcontract.ChunkMatch, error) {
	s.searchCalls = append(s.searchCalls, params)
	if s.searchErr != nil {
		return nil, s.searchErr
	}
	return append([]ragcontract.ChunkMatch(nil), s.matches...), nil
}

type stubEmbedder struct {
	embedding []byte
	err       error
	calls     []string
}

func (s *stubEmbedder) EmbedQuery(_ context.Context, query string) ([]byte, error) {
	s.calls = append(s.calls, query)
	if s.err != nil {
		return nil, s.err
	}
	return append([]byte(nil), s.embedding...), nil
}

type stubReranker struct {
	results []ragcontract.ChunkMatch
	err     error
	calls   []rerankCall
}

type rerankCall struct {
	query      string
	candidates []ragcontract.ChunkMatch
	topK       int64
}

func (s *stubReranker) Rerank(_ context.Context, query string, candidates []ragcontract.ChunkMatch, topK int64) ([]ragcontract.ChunkMatch, error) {
	s.calls = append(s.calls, rerankCall{
		query:      query,
		candidates: append([]ragcontract.ChunkMatch(nil), candidates...),
		topK:       topK,
	})
	if s.err != nil {
		return nil, s.err
	}
	return append([]ragcontract.ChunkMatch(nil), s.results...), nil
}

func newStubEmbeddingClient() *embeddingClient {
	return newStubEmbeddingClientWithState(&stubEmbedder{})
}

func newStubEmbeddingClientWithState(state *stubEmbedder) *embeddingClient {
	return &embeddingClient{
		embedFunc: state.EmbedQuery,
	}
}

func newStubRerankClient() *rerankClient {
	return newStubRerankClientWithState(&stubReranker{})
}

func newStubRerankClientWithState(state *stubReranker) *rerankClient {
	return &rerankClient{
		rerankFunc: state.Rerank,
	}
}

func validRAGConfig() Config {
	return Config{
		APIKey:             "rag-key",
		EmbeddingBaseURL:   "https://rag.example.com/v1/embeddings",
		EmbeddingDimension: 1024,
		EmbeddingModel:     "BAAI/bge-large-zh-v1.5",
		RerankBaseURL:      "https://rag.example.com/v1/rerank",
		RerankModel:        "BAAI/bge-reranker-v2-m3",
	}
}
