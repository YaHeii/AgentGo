package ui

import (
	"context"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/lifecycle"
)

type ChatService struct {
	app *app.APPService
}

func NewChatService(appSvc *app.APPService) *ChatService {
	return &ChatService{app: appSvc}
}

func (s *ChatService) EnsureActiveSession(ctx context.Context) error {
	return s.app.EnsureActiveSession(ctx)
}

func (s *ChatService) RunQuery(ctx context.Context, sessionID string, prompt string) error {
	return s.app.RunQuery(ctx, sessionID, prompt)
}

func (s *ChatService) Events() <-chan app.Event {
	return s.app.Events()
}

func (s *ChatService) InitializePermissionLevel(_ context.Context) error {
	lifecycle.SetPermissionLevel(lifecycle.SafeLevel)
	return nil
}
