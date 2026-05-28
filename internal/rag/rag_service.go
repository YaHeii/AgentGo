package rag

type Config struct {
	APIKey             string
	EmbeddingBaseURL   string
	EmbeddingDimension int
	EmbeddingModel     string
	RerankBaseURL      string
	RerankModel        string
}

type Service struct {
	store ragStore
	cfg   Config
}

func NewService(store ragStore, cfg Config) *Service {
	return &Service{
		store: store,
		cfg:   cfg,
	}
}
