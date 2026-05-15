package session

import (
	"context"

	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
)

type messageStore interface {
	ListMessages(ctx context.Context, sessionID string) ([]messagecontract.Message, error)
	CreateMessage(ctx context.Context, params messagecontract.CreateMessageParams) (messagecontract.Message, error)
}

type sessionStore interface {
	CreateSession(ctx context.Context, params sessioncontract.CreateSessionParams) (sessioncontract.Session, error)
	ListSessions(ctx context.Context) ([]sessioncontract.Session, error)
	GetSession(ctx context.Context, id string) (sessioncontract.Session, error)
	UpdateSession(ctx context.Context, params sessioncontract.UpdateSessionParams) (sessioncontract.Session, error)
	DeleteSession(ctx context.Context, id string) error
}
