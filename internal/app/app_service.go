package app

import (
	"context"
	"errors"

	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/YaHeii/agentGo/internal/tool"
)

type APPService struct {
	sessions   sessionStore
	agent      agentStore
	tools      toolStore
	dispatcher Dispatcher
}

func NewService(sessions sessionStore, agent agentStore, tools toolStore, dispatcher Dispatcher) *APPService {

	return &APPService{
		sessions:   sessions,
		agent:      agent,
		tools:      tools,
		dispatcher: dispatcher,
	}
}

// TODO: deletethis
func (s *APPService) EnsureActiveSession(ctx context.Context) error {
	sessionID, err := s.sessions.GetLast(ctx)
	if err != nil {
		if !errors.Is(err, store.ErrSessionNotFound) {
			return err
		}
		//TODO: title should from AI
		sessionID, err = s.sessions.Create(ctx, "New Session", s.dispatcher)
		if err != nil {
			return err
		}
	}

	return s.sessions.Restore(ctx, sessionID, s.dispatcher)
}

func (s *APPService) CreateSession(ctx context.Context, title string) (string, error) {
	sessionID, err := s.sessions.Create(ctx, title, s.dispatcher)
	return sessionID, err
}

func (s *APPService) ListSessions(ctx context.Context) ([]store.Session, error) {
	return s.sessions.List(ctx)
}

func (s *APPService) RenameSession(ctx context.Context, sessionID string, title string) (store.Session, error) {
	return s.sessions.Rename(ctx, sessionID, title, s.dispatcher)
}

func (s *APPService) SwitchSession(ctx context.Context, sessionID string) error {
	return s.sessions.SwitchSession(ctx, sessionID, s.dispatcher)
}

func (s *APPService) GetSessionID() string {
	return s.sessions.GetSessionID()
}

func (s *APPService) GetParentSessionID() string {
	return s.sessions.GetParentSessionID()
}

func (s *APPService) ListTools(ctx context.Context, permissionLevel tool.SecurityLevel) []tool.Metadata {
	if s.tools == nil {
		return nil
	}
	return s.tools.ListTools(ctx, permissionLevel)
}

func (s *APPService) DeleteSession(ctx context.Context, sessionID string) error {
	return s.sessions.Delete(ctx, sessionID, s.dispatcher)
}

func (s *APPService) ListHistory(ctx context.Context, sessionID string) ([]message.Message, error) {
	return s.sessions.ListHistory(ctx, sessionID, s.dispatcher)
}

func (s *APPService) CallTools(ctx context.Context, req tool.BatchRequest) ([]tool.ToolResult, error) {
	if s.tools == nil {
		return nil, nil
	}
	return s.tools.Call(ctx, req)
}

func (s *APPService) CreateMessage(ctx context.Context, params message.CreateMessageParams) (message.Message, error) {
	return s.sessions.CreateMessage(ctx, params, s.dispatcher)
}

func (s *APPService) SendMessage(ctx context.Context, sessionID string, prompt string) error {
	return s.agent.RunQuery(ctx, sessionID, prompt)
}

func (s *APPService) Events() <-chan Event {
	return s.dispatcher.Subscribe(context.Background())
}
