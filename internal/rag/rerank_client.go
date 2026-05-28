package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	ragcontract "github.com/YaHeii/agentGo/internal/rag/contract"
)

type rerankClient struct {
	client     *http.Client
	cfg        Config
	rerankFunc func(ctx context.Context, query string, candidates []ragcontract.ChunkMatch, topK int64) ([]ragcontract.ChunkMatch, error)
}

type rerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int64    `json:"top_n"`
}

type rerankResponse struct {
	Results []struct {
		Index int `json:"index"`
	} `json:"results"`
}

func newRerankClient(client *http.Client, cfg Config) *rerankClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &rerankClient{
		client: client,
		cfg:    cfg,
	}
}

func (c *rerankClient) Rerank(ctx context.Context, query string, candidates []ragcontract.ChunkMatch, topK int64) ([]ragcontract.ChunkMatch, error) {
	if c.rerankFunc != nil {
		return c.rerankFunc(ctx, query, candidates, topK)
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	documents := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		documents = append(documents, candidate.Chunk.Content)
	}

	reqBody, err := json.Marshal(rerankRequest{
		Model:     c.cfg.RerankModel,
		Query:     query,
		Documents: documents,
		TopN:      topK,
	})
	if err != nil {
		return nil, fmt.Errorf("rag: marshal rerank request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.RerankBaseURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("rag: create rerank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.cfg.APIKey))

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rag: send rerank request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("rag: read rerank response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errUnexpectedStatus(resp.StatusCode, string(body))
	}

	var parsed rerankResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("rag: decode rerank response: %w", err)
	}

	return applyRerankResults(candidates, parsed.Results), nil
}

func applyRerankResults(candidates []ragcontract.ChunkMatch, results []struct {
	Index int `json:"index"`
}) []ragcontract.ChunkMatch {
	if len(results) == 0 {
		return nil
	}

	ranked := make([]ragcontract.ChunkMatch, 0, len(results))
	seen := make(map[int]struct{}, len(results))
	for _, result := range results {
		if result.Index < 0 || result.Index >= len(candidates) {
			continue
		}
		if _, ok := seen[result.Index]; ok {
			continue
		}
		seen[result.Index] = struct{}{}
		ranked = append(ranked, candidates[result.Index])
	}
	return ranked
}
