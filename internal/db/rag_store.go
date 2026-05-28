package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	ragcontract "github.com/YaHeii/agentGo/internal/rag/contract"
)

func (s *Store) UpsertDocument(ctx context.Context, params ragcontract.UpsertDocumentParams) (ragcontract.Document, error) {
	return upsertDocumentWithQuerier(ctx, s.q, params)
}

func (s *Store) GetDocumentBySourcePath(ctx context.Context, sourcePath string) (ragcontract.Document, error) {
	return getDocumentBySourcePathWithQuerier(ctx, s.q, sourcePath)
}

func (s *Store) DeleteDocumentBySourcePath(ctx context.Context, sourcePath string) error {
	return deleteDocumentBySourcePathWithQuerier(ctx, s.q, sourcePath)
}

func (s *Store) CreateChunk(ctx context.Context, params ragcontract.CreateChunkParams) (ragcontract.Chunk, error) {
	return createChunkWithQuerier(ctx, s.q, params)
}

func (s *Store) ListChunksByDocumentID(ctx context.Context, documentID int64) ([]ragcontract.Chunk, error) {
	return listChunksByDocumentIDWithQuerier(ctx, s.q, documentID)
}

func (s *Store) DeleteChunksByDocumentID(ctx context.Context, documentID int64) error {
	return deleteChunksByDocumentIDWithQuerier(ctx, s.q, documentID)
}

func (s *Store) SearchChunksByPrefix(ctx context.Context, params ragcontract.SearchChunksParams) ([]ragcontract.ChunkMatch, error) {
	return searchChunksByPrefixWithQuerier(ctx, s.q, params)
}

func (s *txStore) UpsertDocument(ctx context.Context, params ragcontract.UpsertDocumentParams) (ragcontract.Document, error) {
	return upsertDocumentWithQuerier(ctx, s.q, params)
}

func (s *txStore) GetDocumentBySourcePath(ctx context.Context, sourcePath string) (ragcontract.Document, error) {
	return getDocumentBySourcePathWithQuerier(ctx, s.q, sourcePath)
}

func (s *txStore) DeleteDocumentBySourcePath(ctx context.Context, sourcePath string) error {
	return deleteDocumentBySourcePathWithQuerier(ctx, s.q, sourcePath)
}

func (s *txStore) CreateChunk(ctx context.Context, params ragcontract.CreateChunkParams) (ragcontract.Chunk, error) {
	return createChunkWithQuerier(ctx, s.q, params)
}

func (s *txStore) ListChunksByDocumentID(ctx context.Context, documentID int64) ([]ragcontract.Chunk, error) {
	return listChunksByDocumentIDWithQuerier(ctx, s.q, documentID)
}

func (s *txStore) DeleteChunksByDocumentID(ctx context.Context, documentID int64) error {
	return deleteChunksByDocumentIDWithQuerier(ctx, s.q, documentID)
}

func (s *txStore) SearchChunksByPrefix(ctx context.Context, params ragcontract.SearchChunksParams) ([]ragcontract.ChunkMatch, error) {
	return searchChunksByPrefixWithQuerier(ctx, s.q, params)
}

type ragQuerier interface {
	UpsertDocument(ctx context.Context, arg UpsertDocumentParams) (Document, error)
	GetDocumentBySourcePath(ctx context.Context, sourcePath string) (Document, error)
	DeleteDocumentBySourcePath(ctx context.Context, sourcePath string) (int64, error)
	CreateChunk(ctx context.Context, arg CreateChunkParams) (Chunk, error)
	ListChunksByDocumentID(ctx context.Context, documentID int64) ([]Chunk, error)
	DeleteChunksByDocumentID(ctx context.Context, documentID int64) (int64, error)
	SearchChunksByPrefix(ctx context.Context, arg SearchChunksByPrefixParams) ([]SearchChunksByPrefixRow, error)
}

func upsertDocumentWithQuerier(ctx context.Context, q ragQuerier, params ragcontract.UpsertDocumentParams) (ragcontract.Document, error) {
	row, err := q.UpsertDocument(ctx, UpsertDocumentParams{
		SourcePath:     params.SourcePath,
		NormalizedPath: params.NormalizedPath,
		FileHash:       params.FileHash,
		UpdatedAt:      params.UpdatedAt.UTC().UnixMilli(),
	})
	if err != nil {
		return ragcontract.Document{}, err
	}
	return ragcontract.Document{
		ID:             row.ID,
		SourcePath:     row.SourcePath,
		NormalizedPath: row.NormalizedPath,
		FileHash:       row.FileHash,
		UpdatedAt:      unixMilliToTime(row.UpdatedAt),
	}, nil
}

func getDocumentBySourcePathWithQuerier(ctx context.Context, q ragQuerier, sourcePath string) (ragcontract.Document, error) {
	row, err := q.GetDocumentBySourcePath(ctx, sourcePath)
	if errors.Is(err, sql.ErrNoRows) {
		return ragcontract.Document{}, ragcontract.ErrDocumentNotFound
	}
	if err != nil {
		return ragcontract.Document{}, err
	}
	return ragcontract.Document{
		ID:             row.ID,
		SourcePath:     row.SourcePath,
		NormalizedPath: row.NormalizedPath,
		FileHash:       row.FileHash,
		UpdatedAt:      unixMilliToTime(row.UpdatedAt),
	}, nil
}

func deleteDocumentBySourcePathWithQuerier(ctx context.Context, q ragQuerier, sourcePath string) error {
	rows, err := q.DeleteDocumentBySourcePath(ctx, sourcePath)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ragcontract.ErrDocumentNotFound
	}
	return nil
}

func createChunkWithQuerier(ctx context.Context, q ragQuerier, params ragcontract.CreateChunkParams) (ragcontract.Chunk, error) {
	row, err := q.CreateChunk(ctx, CreateChunkParams{
		DocumentID: params.DocumentID,
		ChunkIndex: params.ChunkIndex,
		Content:    params.Content,
		Embedding:  params.Embedding,
	})
	if err != nil {
		return ragcontract.Chunk{}, err
	}
	return ragcontract.Chunk{
		ID:         row.ID,
		DocumentID: row.DocumentID,
		ChunkIndex: row.ChunkIndex,
		Content:    row.Content,
		Embedding:  append([]byte(nil), row.Embedding...),
	}, nil
}

func listChunksByDocumentIDWithQuerier(ctx context.Context, q ragQuerier, documentID int64) ([]ragcontract.Chunk, error) {
	rows, err := q.ListChunksByDocumentID(ctx, documentID)
	if err != nil {
		return nil, err
	}
	chunks := make([]ragcontract.Chunk, 0, len(rows))
	for _, row := range rows {
		chunks = append(chunks, ragcontract.Chunk{
			ID:         row.ID,
			DocumentID: row.DocumentID,
			ChunkIndex: row.ChunkIndex,
			Content:    row.Content,
			Embedding:  append([]byte(nil), row.Embedding...),
		})
	}
	return chunks, nil
}

func deleteChunksByDocumentIDWithQuerier(ctx context.Context, q ragQuerier, documentID int64) error {
	_, err := q.DeleteChunksByDocumentID(ctx, documentID)
	return err
}

func searchChunksByPrefixWithQuerier(ctx context.Context, q ragQuerier, params ragcontract.SearchChunksParams) ([]ragcontract.ChunkMatch, error) {
	rows, err := q.SearchChunksByPrefix(ctx, SearchChunksByPrefixParams{
		NormalizedPathGlob: params.NormalizedPathGlob,
		QueryEmbedding:     params.QueryEmbedding,
		TopK:               params.TopK,
	})
	if err != nil {
		return nil, err
	}
	matches := make([]ragcontract.ChunkMatch, 0, len(rows))
	for _, row := range rows {
		matches = append(matches, ragcontract.ChunkMatch{
			Chunk: ragcontract.Chunk{
				ID:         row.ID,
				DocumentID: row.DocumentID,
				ChunkIndex: row.ChunkIndex,
				Content:    row.Content,
				Embedding:  append([]byte(nil), row.Embedding...),
			},
			Document: ragcontract.Document{
				ID:             row.DocumentIDAlias,
				SourcePath:     row.SourcePath,
				NormalizedPath: row.NormalizedPath,
				FileHash:       row.FileHash,
				UpdatedAt:      unixMilliToTime(row.UpdatedAt),
			},
			Distance: mustFloat64(row.Distance),
		})
	}
	return matches, nil
}

func mustFloat64(v any) float64 {
	switch typed := v.(type) {
	case float64:
		return typed
	case int64:
		return float64(typed)
	case []byte:
		f, err := strconv.ParseFloat(string(typed), 64)
		if err == nil {
			return f
		}
	case string:
		f, err := strconv.ParseFloat(typed, 64)
		if err == nil {
			return f
		}
	}
	panic(fmt.Sprintf("unexpected distance type %T", v))
}
