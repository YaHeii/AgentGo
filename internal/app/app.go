package app

import (
	"context"
)

type sessionStore interface {
	Create(ctx context.Context, title string, d Dispatcher) (string, error)
	GetLast(ctx context.Context) (string, error)
	Restore(ctx context.Context, sessionID string, d Dispatcher) error
	Delete(ctx context.Context, id string, d Dispatcher) error
}

type agentStore interface {
	RunPrompt(ctx context.Context, sessionID string, prompt string) error
}
