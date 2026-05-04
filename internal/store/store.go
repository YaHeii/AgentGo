package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSessionNotFound = errors.New("store: session not found")
	ErrMessageNotFound = errors.New("store: message not found")
)

type MessageStatus string

const (
	MessageStatusComplete  MessageStatus = "complete"
	MessageStatusStreaming MessageStatus = "streaming"
	MessageStatusCancelled MessageStatus = "cancelled"
	MessageStatusFailed    MessageStatus = "failed"
)

type Session struct {
	ID           string
	Title        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastActiveAt time.Time
}

type Message struct {
	ID        string
	SessionID string
	Role      string
	Content   string
	Status    MessageStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateSessionParams struct {
	ID           string
	Title        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastActiveAt time.Time
}

type UpdateSessionParams struct {
	ID           string
	Title        string
	UpdatedAt    time.Time
	LastActiveAt time.Time
}

type CreateMessageParams struct {
	ID        string
	SessionID string
	Role      string
	Content   string
	Status    MessageStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UpdateMessageParams struct {
	ID        string
	Content   string
	Status    MessageStatus
	UpdatedAt time.Time
}

type SaveDraftParams struct {
	SessionID string
	Content   string
	UpdatedAt time.Time
}

type SessionStore interface {
	CreateSession(ctx context.Context, params CreateSessionParams) (Session, error)
	ListSessions(ctx context.Context) ([]Session, error)
	GetSession(ctx context.Context, id string) (Session, error)
	UpdateSession(ctx context.Context, params UpdateSessionParams) (Session, error)
	DeleteSession(ctx context.Context, id string) error
}

type MessageStore interface {
	CreateMessage(ctx context.Context, params CreateMessageParams) (Message, error)
	ListMessages(ctx context.Context, sessionID string) ([]Message, error)
	UpdateMessage(ctx context.Context, params UpdateMessageParams) (Message, error)
}

type DraftStore interface {
	LoadDraft(ctx context.Context, sessionID string) (string, error)
	SaveDraft(ctx context.Context, params SaveDraftParams) error
	DeleteDraft(ctx context.Context, sessionID string) error
}

type Store interface {
	SessionStore
	MessageStore
	DraftStore
	Close() error
}
