package message

import (
	"context"
	"time"
)

type messageStore interface {
	CreateMessage(ctx context.Context, params CreateMessageRecordParams) (MessageRecord, error)
	ListMessages(ctx context.Context, sessionID string) ([]MessageRecord, error)
	GetMessage(ctx context.Context, id string) (MessageRecord, error)
	DeleteMessage(ctx context.Context, id string) error
	DeleteSessionMessages(ctx context.Context, sessionID string) error
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
