package session

import (
	"context"

	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/store"
)

type messageStore interface {
	ListMessages(ctx context.Context, sessionID string) ([]message.Message, error)
	CreateMessage(ctx context.Context, params message.CreateMessageParams) (message.Message, error)
}

type sessionStore interface {
	CreateSession(ctx context.Context, params store.CreateSessionParams) (store.Session, error)
	ListSessions(ctx context.Context) ([]store.Session, error)
	GetSession(ctx context.Context, id string) (store.Session, error)
	UpdateSession(ctx context.Context, params store.UpdateSessionParams) (store.Session, error)
	DeleteSession(ctx context.Context, id string) error
}
