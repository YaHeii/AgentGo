package app

import (
	"context"
	"errors"

	"github.com/YaHeii/agentGo/internal/store"
)

type APPService struct {
	sessions   sessionStore
	agent      agentStore
	dispatcher Dispatcher
}

func NewService(sessions sessionStore, agent agentStore, dispatcher Dispatcher) *APPService {

	return &APPService{
		sessions: sessions,
		agent:    agent,
		dispatcher: dispatcher,
	}
}

func (s *APPService) EnsureActiveSession(ctx context.Context) error {
	sessionID, err := s.sessions.GetLast(ctx)
	if err != nil {
		if !errors.Is(err, store.ErrSessionNotFound) {
			return err
		}
		//TODO: title should from AI
		sessionID, err = s.sessions.Create(ctx, "New Session",s.dispatcher)
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

func (s *APPService) DeleteSession(ctx context.Context, sessionID string) error {
	return s.sessions.Delete(ctx, sessionID, s.dispatcher)
}

func (s *APPService) SendMessage(ctx context.Context, sessionID string, prompt string) error {
	return s.agent.RunPrompt(ctx, sessionID, prompt)
}
