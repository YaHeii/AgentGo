package app

import (
	"context"

	"github.com/YaHeii/agentGo/internal/store"
)

type sessionStore interface {
	Create(ctx context.Context, title string) (store.Session, error)
	GetLast(ctx context.Context) (store.Session, error)
	Restore(ctx context.Context, sessionID string) error
	Delete(ctx context.Context, sessionID string) error
}

type agentStore interface {
	RunPrompt(ctx context.Context, sessionID string, prompt string) error
}
