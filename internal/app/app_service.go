package app

import (
	"context"

	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
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

func (s *APPService) StartNewSession(ctx context.Context, title string) error {
	sessionID, err := s.sessions.Create(ctx, title, s.dispatcher)
	if err != nil {
		return err
	}
	return s.sessions.SwitchSession(ctx, sessionID, s.dispatcher)
}

func (s *APPService) CreateSession(ctx context.Context, title string) (string, error) {
	sessionID, err := s.sessions.Create(ctx, title, s.dispatcher)
	return sessionID, err
}

func (s *APPService) ListSessions(ctx context.Context) ([]sessioncontract.Session, error) {
	return s.sessions.List(ctx)
}

func (s *APPService) RenameSession(ctx context.Context, sessionID string, title string) (sessioncontract.Session, error) {
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

func (s *APPService) ListTools(ctx context.Context, permissionLevel toolcontract.SecurityLevel) []toolcontract.Metadata {
	if s.tools == nil {
		return nil
	}
	return s.tools.ListTools(ctx, permissionLevel)
}

func (s *APPService) DeleteSession(ctx context.Context, sessionID string) error {
	return s.sessions.Delete(ctx, sessionID, s.dispatcher)
}

func (s *APPService) ListHistory(ctx context.Context, sessionID string) ([]messagecontract.Message, error) {
	return s.sessions.ListHistory(ctx, sessionID, s.dispatcher)
}

func (s *APPService) CallTools(ctx context.Context, req toolcontract.BatchRequest) ([]toolcontract.ToolResult, error) {
	if s.tools == nil {
		return nil, nil
	}
	return s.tools.Call(ctx, req)
}

func (s *APPService) CreateMessage(ctx context.Context, params messagecontract.CreateMessageParams) (messagecontract.Message, error) {
	return s.sessions.CreateMessage(ctx, params, s.dispatcher)
}

func (s *APPService) RunQuery(ctx context.Context, sessionID string, prompt string) error {
	return s.agent.RunQuery(ctx, sessionID, prompt)
}

func (s *APPService) Events() <-chan Event {
	return s.dispatcher.Subscribe(context.Background())
}
