package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/YaHeii/agentGo/internal/store"
)

func (s *Store) LoadDraft(ctx context.Context, sessionID string) (string, error) {
	return loadDraftWithQuerier(ctx, s.q, sessionID)
}

func (s *Store) SaveDraft(ctx context.Context, params store.SaveDraftParams) error {
	return saveDraftWithQuerier(ctx, s.q, params)
}

func (s *Store) DeleteDraft(ctx context.Context, sessionID string) error {
	return deleteDraftWithQuerier(ctx, s.q, sessionID)
}

func (s *txStore) LoadDraft(ctx context.Context, sessionID string) (string, error) {
	return loadDraftWithQuerier(ctx, s.q, sessionID)
}

func (s *txStore) SaveDraft(ctx context.Context, params store.SaveDraftParams) error {
	return saveDraftWithQuerier(ctx, s.q, params)
}

func (s *txStore) DeleteDraft(ctx context.Context, sessionID string) error {
	return deleteDraftWithQuerier(ctx, s.q, sessionID)
}

type draftQuerier interface {
	GetDraft(ctx context.Context, sessionID string) (string, error)
	UpsertDraft(ctx context.Context, arg UpsertDraftParams) error
	DeleteDraft(ctx context.Context, sessionID string) (int64, error)
}

func loadDraftWithQuerier(ctx context.Context, q draftQuerier, sessionID string) (string, error) {
	content, err := q.GetDraft(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return content, nil
}

func saveDraftWithQuerier(ctx context.Context, q draftQuerier, params store.SaveDraftParams) error {
	return q.UpsertDraft(ctx, UpsertDraftParams{
		SessionID: params.SessionID,
		Content:   params.Content,
		UpdatedAt: params.UpdatedAt.UTC().UnixMilli(),
	})
}

func deleteDraftWithQuerier(ctx context.Context, q draftQuerier, sessionID string) error {
	_, err := q.DeleteDraft(ctx, sessionID)
	return err
}
