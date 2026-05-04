package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/YaHeii/agentGo/internal/store"
)

func (s *Store) LoadDraft(ctx context.Context, sessionID string) (string, error) {
	content, err := s.q.GetDraft(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return content, nil
}

func (s *Store) SaveDraft(ctx context.Context, params store.SaveDraftParams) error {
	return s.q.UpsertDraft(ctx, UpsertDraftParams{
		SessionID: params.SessionID,
		Content:   params.Content,
		UpdatedAt: params.UpdatedAt.UTC().UnixMilli(),
	})
}

func (s *Store) DeleteDraft(ctx context.Context, sessionID string) error {
	_, err := s.q.DeleteDraft(ctx, sessionID)
	return err
}
