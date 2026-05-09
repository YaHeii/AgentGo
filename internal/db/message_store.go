package db

import (
	"context"

	"github.com/YaHeii/agentGo/internal/store"
)

func (s *Store) CreateMessage(ctx context.Context, params store.CreateMessageParams) (store.Message, error) {
	return createMessageWithQuerier(ctx, s.q, params)
}

func (s *Store) ListMessages(ctx context.Context, sessionID string) ([]store.Message, error) {
	return listMessagesWithQuerier(ctx, s.q, sessionID)
}

func (s *txStore) CreateMessage(ctx context.Context, params store.CreateMessageParams) (store.Message, error) {
	return createMessageWithQuerier(ctx, s.q, params)
}

func (s *txStore) ListMessages(ctx context.Context, sessionID string) ([]store.Message, error) {
	return listMessagesWithQuerier(ctx, s.q, sessionID)
}

type messageQuerier interface {
	CreateMessage(ctx context.Context, arg CreateMessageParams) (Message, error)
	ListMessages(ctx context.Context, sessionID string) ([]Message, error)
}

func createMessageWithQuerier(ctx context.Context, q messageQuerier, params store.CreateMessageParams) (store.Message, error) {
	row, err := q.CreateMessage(ctx, CreateMessageParams{
		ID:               params.ID,
		SessionID:        params.SessionID,
		Kind:             params.Kind,
		Provider:         params.Provider,
		FinishedAt:       params.FinishedAt.UTC().UnixMilli(),
		IsCompactSummary: boolToInt64(params.IsCompactSummary),
		MessageJson:      params.MessageJSON,
	})
	if err != nil {
		return store.Message{}, err
	}

	return store.Message{
		ID:               row.ID,
		SessionID:        row.SessionID,
		Kind:             row.Kind,
		Provider:         row.Provider,
		FinishedAt:       unixMilliToTime(row.FinishedAt),
		IsCompactSummary: int64ToBool(row.IsCompactSummary),
		MessageJSON:      row.MessageJson,
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
			ID:               row.ID,
			SessionID:        row.SessionID,
			Kind:             row.Kind,
			Provider:         row.Provider,
			FinishedAt:       unixMilliToTime(row.FinishedAt),
			IsCompactSummary: int64ToBool(row.IsCompactSummary),
			MessageJSON:      row.MessageJson,
		})
	}

	return messages, nil
}

func boolToInt64(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func int64ToBool(v int64) bool {
	return v != 0
}
