package app

import (
	"context"
	"time"

	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/store"
)

// WIP：as a facade to multi service

type Store interface{ store.Store }

type Service struct {
	store    Store
	bus      bus.Bus[Event]
	events   <-chan Event
	messages message.Service
	query    agent.QueryRunner
	nowFunc  func() time.Time
}

func NewService(st Store, llm provider.StreamingLLM, nowFunc func() time.Time) *Service {
	if nowFunc == nil {
		nowFunc = time.Now
	}

	appBus := bus.NewBus[Event](128)
	messages := message.NewMessageService(st)
	query := agent.NewMessageQueryRunner(messages, llm)

	svc := &Service{
		store:    st,
		bus:      appBus,
		events:   appBus.Subscribe(context.Background()),
		nowFunc:  nowFunc,
		messages: messages,
		query:    query,
	}

	go svc.forwardMessageEvents(messages.Events())

	return svc
}

func (s *Service) Events() <-chan Event {
	return s.events
}

func (s *Service) EnsureActiveSession(ctx context.Context) (Session, error) {
	sessions, err := s.store.ListSessions(ctx)
	if err != nil {
		return Session{}, err
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
			return Session{}, err
		}
	} else {
		session = sessions[0]
	}

	messages, err := s.store.ListMessages(ctx, session.ID)
	if err != nil {
		return Session{}, err
	}

	s.bus.Publish(SessionReadyEvent{Session: toAppSession(session)})
	s.bus.Publish(ConversationHydratedEvent{
		SessionID: session.ID,
		Messages:  toAppMessages(messages),
	})

	return toAppSession(session), nil
}

func (s *Service) SendMessage(ctx context.Context, params SendMessageParams) (SendMessageResult, error) {
	result, err := s.query.RunQuery(ctx, agent.QueryParams{
		SessionID: params.SessionID,
		Prompt:    params.Prompt,
	})
	if err != nil {
		return SendMessageResult{
			User:      Message{ID: result.UserMessageID},
			Assistant: Message{ID: result.AssistantMessageID},
		}, err
	}

	return SendMessageResult{
		User:      Message{ID: result.UserMessageID},
		Assistant: Message{ID: result.AssistantMessageID},
	}, nil
}

func (s *Service) forwardMessageEvents(events <-chan message.Event) {
	for evt := range events {
		switch event := evt.(type) {
		case message.MessageCreatedEvent:
			s.bus.Publish(MessageCreatedEvent{Message: toAppMessage(event.Message)})
		case message.MessageDeltaEvent:
			s.bus.Publish(MessageDeltaEvent{
				Message: toAppMessage(event.Message),
				Delta:   event.Delta,
			})
		case message.MessageCompletedEvent:
			s.bus.Publish(MessageCompletedEvent{Message: toAppMessage(event.Message)})
		case message.MessageFailedEvent:
			s.bus.Publish(MessageFailedEvent{
				Message: toAppMessage(event.Message),
				Err:     event.Err,
			})
		case message.MessageCancelledEvent:
			s.bus.Publish(MessageCancelledEvent{
				Message: toAppMessage(event.Message),
				Err:     event.Err,
			})
		}
	}
}

func toAppSession(session store.Session) Session {
	return Session{
		ID:           session.ID,
		Title:        session.Title,
		CreatedAt:    session.CreatedAt,
		UpdatedAt:    session.UpdatedAt,
		LastActiveAt: session.LastActiveAt,
	}
}

func toAppMessages(messages []store.Message) []Message {
	out := make([]Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, Message{
			ID:        msg.ID,
			SessionID: msg.SessionID,
			Role:      msg.Role,
			Content:   msg.Content,
			Status:    MessageStatus(msg.Status),
			CreatedAt: msg.CreatedAt,
			UpdatedAt: msg.UpdatedAt,
		})
	}

	return out
}

func toAppMessage(msg message.Message) Message {
	return Message{
		ID:        msg.ID,
		SessionID: msg.SessionID,
		Role:      toAppRole(msg.Kind),
		Content:   toAppContent(msg),
		Status:    MessageStatus(msg.Status),
		CreatedAt: msg.CreatedAt,
		UpdatedAt: msg.UpdatedAt,
	}
}

func toAppRole(kind message.Kind) string {
	switch kind {
	case message.KindAssistant:
		return "assistant"
	case message.KindUser:
		return "user"
	default:
		return "system"
	}
}

func toAppContent(msg message.Message) string {
	for _, part := range msg.Parts {
		if part.Type == message.PartTypeText {
			return part.Text
		}
	}
	return ""
}
