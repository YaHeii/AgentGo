package message

import (
	"context"
	"time"
)

type messageStore interface {
	CreateMessage(ctx context.Context, params CreateMessageRecordParams) (MessageRecord, error)
	ListMessages(ctx context.Context, sessionID string) ([]MessageRecord, error)
	// TODO: add delete and  SELECT
}

type MessageRecord struct {
	ID               string
	SessionID        string
	Kind             string
	Provider         string
	FinishedAt       time.Time
	IsCompactSummary bool
	MessageJSON      string
}

type CreateMessageRecordParams struct {
	ID               string
	SessionID        string
	Kind             string
	Provider         string
	FinishedAt       time.Time
	IsCompactSummary bool
	MessageJSON      string
}
