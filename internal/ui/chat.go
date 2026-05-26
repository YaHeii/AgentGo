package ui

import (
	"context"

	"github.com/YaHeii/agentGo/internal/app"
	message "github.com/YaHeii/agentGo/internal/message/contract"
)

const (
	defaultWidth  = 80
	defaultHeight = 24
)

type appEventMsg struct {
	event app.Event
}
//ui serface send request
type sendMessageDoneMsg struct {
	err error
}
// ui serface send the request to get the history
type historyLoadedMsg struct {
	sessionID string
	messages  []message.Message
	err       error
}

type appService interface {
	EnsureActiveSession(ctx context.Context) error
	ListHistory(ctx context.Context, sessionID string) ([]message.Message, error)
	RunQuery(ctx context.Context, sessionID string, prompt string) error
	Events() <-chan app.Event
}
