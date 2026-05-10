package app

import (
	"context"
	"errors"

	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/store"
)

type Dependencies struct {
	Sessions sessionStore
	Agent    agentStore
	Bus      bus.Bus[Event]
}

type APPService struct {
	sessions sessionStore
	agent    agentStore
	bus      bus.Bus[Event]
	events   <-chan Event
}

func NewService(deps Dependencies) *APPService {
	eventBus := deps.Bus
	if eventBus == nil {
		eventBus = bus.NewBus[Event](128)
	}

	return &APPService{
		sessions: deps.Sessions,
		agent:    deps.Agent,
		bus:      eventBus,
		events:   eventBus.Subscribe(context.Background()),
	}
}

func (s *APPService) Events() <-chan Event {
	return s.events
}

func (s *APPService) EnsureActiveSession(ctx context.Context) error {
	current, err := s.sessions.GetLast(ctx)
	if err != nil {
		if !errors.Is(err, store.ErrSessionNotFound) {
			return err
		}

		current, err = s.sessions.Create(ctx, "New Session")
		if err != nil {
			return err
		}
	}

	return s.sessions.Restore(ctx, current.ID)
}

func (s *APPService) CreateSession(ctx context.Context, title string) error {
	_, err := s.sessions.Create(ctx, title)
	return err
}

func (s *APPService) DeleteSession(ctx context.Context, sessionID string) error {
	return s.sessions.Delete(ctx, sessionID)
}

func (s *APPService) SendMessage(ctx context.Context, sessionID string, prompt string) error {
	return s.agent.RunPrompt(ctx, sessionID, prompt)
}
