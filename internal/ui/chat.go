package ui

import (
	"context"

	"github.com/YaHeii/agentGo/internal/app"
	message "github.com/YaHeii/agentGo/internal/message/contract"
	"github.com/charmbracelet/glamour"
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

type historyLoadedMsg struct {
	sessionID string
	messages  []message.Message
	err       error
}

type markdownRenderer interface {
	Render(string) (string, error)
}

type defaultMarkdownRenderer struct {
	renderer *glamour.TermRenderer
}

func newDefaultMarkdownRenderer() markdownRenderer {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(defaultWidth),
	)
	if err != nil {
		return nil
	}
	return &defaultMarkdownRenderer{renderer: renderer}
}

func (r *defaultMarkdownRenderer) Render(input string) (string, error) {
	if r == nil || r.renderer == nil {
		return input, nil
	}
	return r.renderer.Render(input)
}

type chatService interface {
	EnsureActiveSession(ctx context.Context) error
	ListHistory(ctx context.Context, sessionID string) ([]message.Message, error)
	RunQuery(ctx context.Context, sessionID string, prompt string) error
	Events() <-chan app.Event
	InitializePermissionLevel(ctx context.Context) error
}
