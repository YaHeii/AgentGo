package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/YaHeii/agentGo/internal/store"
)

func (s *Store) CreateMessage(ctx context.Context, params store.CreateMessageParams) (store.Message, error) {
	return createMessageWithQuerier(ctx, s.q, params)
}

func (s *Store) ListMessages(ctx context.Context, sessionID string) ([]store.Message, error) {
	return listMessagesWithQuerier(ctx, s.q, sessionID)
}

func (s *Store) UpdateMessage(ctx context.Context, params store.UpdateMessageParams) (store.Message, error) {
	return updateMessageWithQuerier(ctx, s.q, params)
}

func (s *txStore) CreateMessage(ctx context.Context, params store.CreateMessageParams) (store.Message, error) {
	return createMessageWithQuerier(ctx, s.q, params)
}

func (s *txStore) ListMessages(ctx context.Context, sessionID string) ([]store.Message, error) {
	return listMessagesWithQuerier(ctx, s.q, sessionID)
}

func (s *txStore) UpdateMessage(ctx context.Context, params store.UpdateMessageParams) (store.Message, error) {
	return updateMessageWithQuerier(ctx, s.q, params)
}

type messageQuerier interface {
	CreateMessage(ctx context.Context, arg CreateMessageParams) (Message, error)
	ListMessages(ctx context.Context, sessionID string) ([]Message, error)
	UpdateMessage(ctx context.Context, arg UpdateMessageParams) (Message, error)
}

func createMessageWithQuerier(ctx context.Context, q messageQuerier, params store.CreateMessageParams) (store.Message, error) {
	row, err := q.CreateMessage(ctx, CreateMessageParams{
		ID:        params.ID,
		SessionID: params.SessionID,
		Role:      params.Role,
		Content:   params.Content,
		Status:    string(params.Status),
		CreatedAt: params.CreatedAt.UTC().UnixMilli(),
		UpdatedAt: params.UpdatedAt.UTC().UnixMilli(),
	})
	if err != nil {
		return store.Message{}, err
	}

	return store.Message{
		ID:        row.ID,
		SessionID: row.SessionID,
		Role:      row.Role,
		Content:   row.Content,
		Status:    store.MessageStatus(row.Status),
		CreatedAt: unixMilliToTime(row.CreatedAt),
		UpdatedAt: unixMilliToTime(row.UpdatedAt),
	}, nil
}

func listMessagesWithQuerier(ctx context.Context, q messageQuerier, sessionID string) ([]store.Message, error) {
	rows, err := q.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	messages := make([]store.Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, store.Message{
			ID:        row.ID,
			SessionID: row.SessionID,
			Role:      row.Role,
			Content:   row.Content,
			Status:    store.MessageStatus(row.Status),
			CreatedAt: unixMilliToTime(row.CreatedAt),
			UpdatedAt: unixMilliToTime(row.UpdatedAt),
		})
	}

	return messages, nil
}

func updateMessageWithQuerier(ctx context.Context, q messageQuerier, params store.UpdateMessageParams) (store.Message, error) {
	row, err := q.UpdateMessage(ctx, UpdateMessageParams{
		Content:   params.Content,
		Status:    string(params.Status),
		UpdatedAt: params.UpdatedAt.UTC().UnixMilli(),
		ID:        params.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return store.Message{}, store.ErrMessageNotFound
	}
	if err != nil {
		return store.Message{}, err
	}

	return store.Message{
		ID:        row.ID,
		SessionID: row.SessionID,
		Role:      row.Role,
		Content:   row.Content,
		Status:    store.MessageStatus(row.Status),
		CreatedAt: unixMilliToTime(row.CreatedAt),
		UpdatedAt: unixMilliToTime(row.UpdatedAt),
	}, nil
}
