package app

import (
	"context"
	"time"

	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/store"
)

type Store interface{ store.Store }

type Service struct {
	store    Store
	bus      bus.EventBus
	messages *message.MessageService
	nowFunc  func() time.Time
}

func NewService(st Store, llm provider.StreamingLLM, nowFunc func() time.Time) *Service {
	if nowFunc == nil {
		nowFunc = time.Now
	}

	bus := bus.NewBus(128)

	return &Service{
		store:    st,
		bus:      bus,
		nowFunc:  nowFunc,
		messages: message.NewMessageService(st, llm, bus, nowFunc),
	}
}

func (s *Service) Events() <-chan Event {
	return s.bus.Events()
}

func (s *Service) EnsureActiveSession(ctx context.Context) (store.Session, error) {
	sessions, err := s.store.ListSessions(ctx)
	if err != nil {
		return store.Session{}, err
	}

	var session store.Session
	if len(sessions) == 0 {
		now := s.nowFunc().UTC()
		session, err = s.store.CreateSession(ctx, store.CreateSessionParams{
			ID:           "session-" + now.Format(time.RFC3339Nano),
			Title:        "New Session",
			CreatedAt:    now,
			UpdatedAt:    now,
			LastActiveAt: now,
		})
		if err != nil {
			return store.Session{}, err
		}
	} else {
		session = sessions[0]
	}

	messages, err := s.store.ListMessages(ctx, session.ID)
	if err != nil {
		return store.Session{}, err
	}

	s.bus.Publish(SessionReadyEvent{Session: session})
	s.bus.Publish(ConversationHydratedEvent{
		SessionID: session.ID,
		Messages:  messages,
	})

	return session, nil
}

func (s *Service) SendMessage(ctx context.Context, params SendMessageParams) (SendMessageResult, error) {
	return s.messages.SendMessage(ctx, params)
}
