package ui

import (
	"context"

	"github.com/YaHeii/agentGo/internal/app"
)

const (
	roleUser      = "user"
	roleAssistant = "assistant"
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
	EnsureActiveSession(ctx context.Context) (app.Session, error)
	SendMessage(ctx context.Context, params app.SendMessageParams) (app.SendMessageResult, error)
	Events() <-chan app.Event
}
