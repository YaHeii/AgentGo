package rag

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	ragcontract "github.com/YaHeii/agentGo/internal/rag/contract"
	"github.com/stretchr/testify/require"
)

func TestRerankClientRerankSendsRequestAndReordersMatches(t *testing.T) {
	t.Parallel()

	candidates := []ragcontract.ChunkMatch{
		{Chunk: ragcontract.Chunk{ID: 1, Content: "alpha"}},
		{Chunk: ragcontract.Chunk{ID: 2, Content: "beta"}},
		{Chunk: ragcontract.Chunk{ID: 3, Content: "gamma"}},
	}

	var gotAuth string
	var gotBody map[string]any
	client := newRerankClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "https://rag.example.com/v1/rerank", r.URL.String())
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"id": "rerank-1",
				"results": [
					{"index": 2, "relevance_score": 1.0},
					{"index": 1, "relevance_score": 1.0},
					{"index": 0, "relevance_score": 0.5}
				]
			}`)),
		}, nil
	})}, validRAGConfig())

	ranked, err := client.Rerank(context.Background(), "hello", candidates, 3)
	require.NoError(t, err)
	require.Equal(t, "Bearer rag-key", gotAuth)
	require.Equal(t, "BAAI/bge-reranker-v2-m3", gotBody["model"])
	require.Equal(t, "hello", gotBody["query"])
	require.Equal(t, []any{"alpha", "beta", "gamma"}, gotBody["documents"])
	require.Equal(t, float64(3), gotBody["top_n"])
	require.Equal(t, []ragcontract.ChunkMatch{candidates[2], candidates[1], candidates[0]}, ranked)
}

func TestRerankClientRerankIgnoresUnknownAndDuplicateResults(t *testing.T) {
	t.Parallel()

	candidates := []ragcontract.ChunkMatch{
		{Chunk: ragcontract.Chunk{ID: 1, Content: "alpha"}},
		{Chunk: ragcontract.Chunk{ID: 2, Content: "beta"}},
		{Chunk: ragcontract.Chunk{ID: 3, Content: "gamma"}},
	}

	client := newRerankClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"id": "rerank-1",
				"results": [
					{"index": 99, "relevance_score": 10},
					{"index": 2, "relevance_score": 0.9},
					{"index": 2, "relevance_score": 0.8},
					{"index": 0, "relevance_score": 0.7}
				]
			}`)),
		}, nil
	})}, validRAGConfig())

	ranked, err := client.Rerank(context.Background(), "hello", candidates, 3)
	require.NoError(t, err)
	require.Equal(t, []ragcontract.ChunkMatch{candidates[2], candidates[0]}, ranked)
}

func TestRerankClientRerankRejectsInvalidResponse(t *testing.T) {
	t.Parallel()

	client := newRerankClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"results":`)),
		}, nil
	})}, validRAGConfig())

	ranked, err := client.Rerank(context.Background(), "hello", []ragcontract.ChunkMatch{
		{Chunk: ragcontract.Chunk{Content: "alpha"}},
	}, 1)
	require.Error(t, err)
	require.Nil(t, ranked)
}
