package message

import (
	"context"
	"time"
)

type Status string

const (
	StatusComplete  Status = "complete"
	StatusStreaming Status = "streaming"
	StatusCancelled Status = "cancelled"
	StatusFailed    Status = "failed"
)

type Message struct {
	ID        string
	SessionID string
	Role      string
	Content   string
	Status    Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SendMessageParams struct {
	SessionID string
	Prompt    string
}

type SendMessageResult struct {
	User      Message
	Assistant Message
}

type Service interface {
	SendMessage(ctx context.Context, params SendMessageParams) (SendMessageResult, error)
	Events() <-chan Event
}
