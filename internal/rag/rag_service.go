package rag

import (
	"context"
	"errors"
	"fmt"
	"strings"

	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	ragcontract "github.com/YaHeii/agentGo/internal/rag/contract"
)

type Config struct {
	APIKey             string
	EmbeddingBaseURL   string
	EmbeddingDimension int
	EmbeddingModel     string
	RerankBaseURL      string
	RerankModel        string
}

type Service struct {
	store           ragStore
	embeddingClient *embeddingClient
	rerankClient    *rerankClient
	cfg             Config
}

func NewService(store ragStore, cfg Config) *Service {
	return newService(store, cfg, newEmbeddingClient(nil, cfg), newRerankClient(nil, cfg))
}

func newService(store ragStore, cfg Config, emb *embeddingClient, rer *rerankClient) *Service {
	return &Service{
		store:           store,
		embeddingClient: emb,
		rerankClient:    rer,
		cfg:             cfg,
	}
}

func (s *Service) Query(ctx context.Context, params ragcontract.QueryParams) (messagecontract.Message, error) {
	if err := validateQueryParams(params); err != nil {
		return messagecontract.Message{}, err
	}
	if err := validateConfig(s.cfg); err != nil {
		return messagecontract.Message{}, err
	}
	if s.store == nil {
		return messagecontract.Message{}, errors.New("rag: store is required")
	}
	if s.embeddingClient == nil {
		return messagecontract.Message{}, errors.New("rag: embedding client is required")
	}
	if s.rerankClient == nil {
		return messagecontract.Message{}, errors.New("rag: rerank client is required")
	}

	query := strings.TrimSpace(params.RawQuery)
	embedding, err := s.embeddingClient.EmbedQuery(ctx, query)
	if err != nil {
		return messagecontract.Message{}, err
	}

	matches, err := s.store.SearchChunksByPrefix(ctx, ragcontract.SearchChunksParams{
		NormalizedPathGlob: params.NormalizedPathGlob,
		QueryEmbedding:     embedding,
		TopK:               recallTopN(params.TopK),
	})
	if err != nil {
		return messagecontract.Message{}, err
	}
	if len(matches) == 0 {
		return assembleContextMessage(nil), nil
	}

	ranked, err := s.rerankClient.Rerank(ctx, query, matches, params.TopK)
	if err != nil {
		return messagecontract.Message{}, err
	}
	if len(ranked) == 0 {
		return assembleContextMessage(nil), nil
	}

	return assembleContextMessage(ranked), nil
}

func validateQueryParams(params ragcontract.QueryParams) error {
	switch {
	case strings.TrimSpace(params.RawQuery) == "":
		return errors.New("rag: raw query is required")
	case strings.TrimSpace(params.NormalizedPathGlob) == "":
		return errors.New("rag: normalized path glob is required")
	case params.TopK <= 0:
		return errors.New("rag: topk must be greater than 0")
	default:
		return nil
	}
}

func validateConfig(cfg Config) error {
	switch {
	case strings.TrimSpace(cfg.APIKey) == "":
		return errors.New("rag: api key is required")
	case strings.TrimSpace(cfg.EmbeddingBaseURL) == "":
		return errors.New("rag: embedding base url is required")
	case strings.TrimSpace(cfg.EmbeddingModel) == "":
		return errors.New("rag: embedding model is required")
	case cfg.EmbeddingDimension <= 0:
		return errors.New("rag: embedding dimension must be greater than 0")
	case strings.TrimSpace(cfg.RerankBaseURL) == "":
		return errors.New("rag: rerank base url is required")
	case strings.TrimSpace(cfg.RerankModel) == "":
		return errors.New("rag: rerank model is required")
	default:
		return nil
	}
}

func recallTopN(topK int64) int64 {
	recall := topK * 4
	if recall < 10 {
		return 10
	}
	if recall > 50 {
		return 50
	}
	return recall
}

func errUnexpectedStatus(statusCode int, body string) error {
	return fmt.Errorf("rag: unexpected status %d: %s", statusCode, strings.TrimSpace(body))
}
