package rag

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddingClientEmbedQuerySendsRequestAndEncodesLittleEndian(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotBody map[string]any
	client := newEmbeddingClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "https://rag.example.com/v1/embeddings", r.URL.String())
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"model": "BAAI/bge-large-zh-v1.5",
				"data": [
					{"embedding": [1.5, -2.25], "index": 0}
				],
				"usage": {"prompt_tokens": 1, "completion_tokens": 0, "total_tokens": 1}
			}`)),
		}, nil
	})}, validRAGConfig())

	embedding, err := client.EmbedQuery(context.Background(), "hello")
	require.NoError(t, err)
	require.Equal(t, "Bearer rag-key", gotAuth)
	require.Equal(t, "BAAI/bge-large-zh-v1.5", gotBody["model"])
	require.Equal(t, "hello", gotBody["input"])
	require.Equal(t, "float", gotBody["encoding_format"])
	require.Len(t, embedding, 8)
	require.InDelta(t, 1.5, float64(math.Float32frombits(binary.LittleEndian.Uint32(embedding[0:4]))), 0.0001)
	require.InDelta(t, -2.25, float64(math.Float32frombits(binary.LittleEndian.Uint32(embedding[4:8]))), 0.0001)
}

func TestEmbeddingClientEmbedQueryRejectsInvalidResponse(t *testing.T) {
	t.Parallel()

	client := newEmbeddingClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"data":[]}`)),
		}, nil
	})}, validRAGConfig())

	embedding, err := client.EmbedQuery(context.Background(), "hello")
	require.Error(t, err)
	require.Nil(t, embedding)
}
