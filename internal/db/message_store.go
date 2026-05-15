package db

import (
	"context"

	"github.com/YaHeii/agentGo/internal/message"
)

func (s *Store) CreateMessage(ctx context.Context, params message.CreateMessageRecordParams) (message.MessageRecord, error) {
	return createMessageWithQuerier(ctx, s.q, params)
}

func (s *Store) ListMessages(ctx context.Context, sessionID string) ([]message.MessageRecord, error) {
	return listMessagesWithQuerier(ctx, s.q, sessionID)
}

func (s *txStore) CreateMessage(ctx context.Context, params message.CreateMessageRecordParams) (message.MessageRecord, error) {
	return createMessageWithQuerier(ctx, s.q, params)
}

func (s *txStore) ListMessages(ctx context.Context, sessionID string) ([]message.MessageRecord, error) {
	return listMessagesWithQuerier(ctx, s.q, sessionID)
}

type messageQuerier interface {
	CreateMessage(ctx context.Context, arg CreateMessageParams) (Message, error)
	ListMessages(ctx context.Context, sessionID string) ([]Message, error)
}

func createMessageWithQuerier(ctx context.Context, q messageQuerier, params message.CreateMessageRecordParams) (message.MessageRecord, error) {
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
		return message.MessageRecord{}, err
	}

	return message.MessageRecord{
		ID:               row.ID,
		SessionID:        row.SessionID,
		Kind:             row.Kind,
		Provider:         row.Provider,
		FinishedAt:       unixMilliToTime(row.FinishedAt),
		IsCompactSummary: int64ToBool(row.IsCompactSummary),
		MessageJSON:      row.MessageJson,
	}, nil
}

func listMessagesWithQuerier(ctx context.Context, q messageQuerier, sessionID string) ([]message.MessageRecord, error) {
	rows, err := q.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	messages := make([]message.MessageRecord, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, message.MessageRecord{
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
