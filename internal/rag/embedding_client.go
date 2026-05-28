package rag

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
)

type embeddingClient struct {
	client    *http.Client
	cfg       Config
	embedFunc func(ctx context.Context, query string) ([]byte, error)
}

type embeddingRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	EncodingFormat string `json:"encoding_format"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func newEmbeddingClient(client *http.Client, cfg Config) *embeddingClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &embeddingClient{
		client: client,
		cfg:    cfg,
	}
}

func (c *embeddingClient) EmbedQuery(ctx context.Context, query string) ([]byte, error) {
	if c.embedFunc != nil {
		return c.embedFunc(ctx, query)
	}

	reqBody, err := json.Marshal(embeddingRequest{
		Model:          c.cfg.EmbeddingModel,
		Input:          query,
		EncodingFormat: "float",
	})
	if err != nil {
		return nil, fmt.Errorf("rag: marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.EmbeddingBaseURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("rag: create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.cfg.APIKey))

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rag: send embedding request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("rag: read embedding response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errUnexpectedStatus(resp.StatusCode, string(body))
	}

	var parsed embeddingResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("rag: decode embedding response: %w", err)
	}
	if len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) == 0 {
		return nil, errors.New("rag: embedding response missing vector")
	}

	out := make([]byte, len(parsed.Data[0].Embedding)*4)
	for i, value := range parsed.Data[0].Embedding {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(value))
	}
	return out, nil
}
