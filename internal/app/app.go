package app

import (
	"context"

	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

type sessionStore interface {
	Create(ctx context.Context, title string, d Dispatcher) (string, error)
	List(ctx context.Context) ([]sessioncontract.Session, error)
	Rename(ctx context.Context, id string, title string, d Dispatcher) (sessioncontract.Session, error)
	Restore(ctx context.Context, sessionID string, d Dispatcher) error
	SwitchSession(ctx context.Context, sessionID string, d Dispatcher) error
	GetSessionID() string
	GetParentSessionID() string
	Delete(ctx context.Context, id string, d Dispatcher) error
	ListHistory(ctx context.Context, sessionID string, d Dispatcher) ([]messagecontract.Message, error)
	CreateMessage(ctx context.Context, params messagecontract.CreateMessageParams, d Dispatcher) (messagecontract.Message, error)
}

type agentStore interface {
	RunQuery(ctx context.Context, sessionID string, prompt string) error
}

type toolStore interface {
	ListTools(ctx context.Context, permissionLevel toolcontract.SecurityLevel) []toolcontract.Metadata
	Call(ctx context.Context, req toolcontract.BatchRequest) ([]toolcontract.ToolResult, error)
}
