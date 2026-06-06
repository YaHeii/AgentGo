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

func (s *Store) GetMessage(ctx context.Context, id string) (message.MessageRecord, error) {
	return getMessageWithQuerier(ctx, s.q, id)
}

func (s *Store) DeleteMessage(ctx context.Context, id string) error {
	return deleteMessageWithQuerier(ctx, s.q, id)
}

func (s *Store) DeleteSessionMessages(ctx context.Context, sessionID string) error {
	return deleteSessionMessagesWithQuerier(ctx, s.q, sessionID)
}

func (s *txStore) CreateMessage(ctx context.Context, params message.CreateMessageRecordParams) (message.MessageRecord, error) {
	return createMessageWithQuerier(ctx, s.q, params)
}

func (s *txStore) ListMessages(ctx context.Context, sessionID string) ([]message.MessageRecord, error) {
	return listMessagesWithQuerier(ctx, s.q, sessionID)
}

func (s *txStore) GetMessage(ctx context.Context, id string) (message.MessageRecord, error) {
	return getMessageWithQuerier(ctx, s.q, id)
}

func (s *txStore) DeleteMessage(ctx context.Context, id string) error {
	return deleteMessageWithQuerier(ctx, s.q, id)
}

func (s *txStore) DeleteSessionMessages(ctx context.Context, sessionID string) error {
	return deleteSessionMessagesWithQuerier(ctx, s.q, sessionID)
}

type messageQuerier interface {
	CreateMessage(ctx context.Context, arg CreateMessageParams) (Message, error)
	ListMessages(ctx context.Context, sessionID string) ([]Message, error)
	GetMessage(ctx context.Context, id string) (Message, error)
	DeleteMessage(ctx context.Context, id string) (int64, error)
	DeleteSessionMessages(ctx context.Context, sessionID string) (int64, error)
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

	return toMessageRecord(row), nil
}

func listMessagesWithQuerier(ctx context.Context, q messageQuerier, sessionID string) ([]message.MessageRecord, error) {
	rows, err := q.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	messages := make([]message.MessageRecord, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageRecord(row))
	}

	return messages, nil
}

func getMessageWithQuerier(ctx context.Context, q messageQuerier, id string) (message.MessageRecord, error) {
	row, err := q.GetMessage(ctx, id)
	if err != nil {
		return message.MessageRecord{}, err
	}
	return toMessageRecord(row), nil
}

func deleteMessageWithQuerier(ctx context.Context, q messageQuerier, id string) error {
	_, err := q.DeleteMessage(ctx, id)
	return err
}

func deleteSessionMessagesWithQuerier(ctx context.Context, q messageQuerier, sessionID string) error {
	_, err := q.DeleteSessionMessages(ctx, sessionID)
	return err
}

func toMessageRecord(row Message) message.MessageRecord {
	return message.MessageRecord{
		ID:               row.ID,
		SessionID:        row.SessionID,
		Kind:             row.Kind,
		Provider:         row.Provider,
		FinishedAt:       unixMilliToTime(row.FinishedAt),
		IsCompactSummary: int64ToBool(row.IsCompactSummary),
		MessageJSON:      row.MessageJson,
	}
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
