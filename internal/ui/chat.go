package ui

import (
	"context"

	"github.com/YaHeii/agentGo/internal/app"
)

const (
	defaultWidth  = 80
	defaultHeight = 24
)

type sendMessageDoneMsg struct {
	err error
}

type appEventMsg struct {
	event app.Event
}

type chatService interface {
	EnsureActiveSession(ctx context.Context) error
	RunQuery(ctx context.Context, sessionID string, prompt string) error
	Events() <-chan app.Event
	InitializePermissionLevel(ctx context.Context) error
}
