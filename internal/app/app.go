package app

import (
	"context"

	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/YaHeii/agentGo/internal/tool"
)

type sessionStore interface {
	Create(ctx context.Context, title string, d Dispatcher) (string, error)
	GetLast(ctx context.Context) (string, error)
	List(ctx context.Context) ([]store.Session, error)
	Rename(ctx context.Context, id string, title string, d Dispatcher) (store.Session, error)
	Restore(ctx context.Context, sessionID string, d Dispatcher) error
	SwitchSession(ctx context.Context, sessionID string, d Dispatcher) error
	GetSessionID() string
	GetParentSessionID() string
	Delete(ctx context.Context, id string, d Dispatcher) error
	ListHistory(ctx context.Context, sessionID string, d Dispatcher) ([]message.Message, error)
	CreateMessage(ctx context.Context, params message.CreateMessageParams, d Dispatcher) (message.Message, error)
}

type agentStore interface {
	RunQuery(ctx context.Context, sessionID string, prompt string) error
}

type toolStore interface {
	ListTools(ctx context.Context, permissionLevel tool.SecurityLevel) []tool.Metadata
	Call(ctx context.Context, req tool.BatchRequest) ([]tool.ToolResult, error)
}
