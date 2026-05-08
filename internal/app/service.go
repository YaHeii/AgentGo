package app

import (
	"context"
	"errors"
	"time"

	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/session"
	"github.com/YaHeii/agentGo/internal/store"
)

// WIP：as a facade to multi service

type Store interface{ store.Store }

type Service struct {
	sessions session.Service
	bus      bus.Bus[Event]
	events   <-chan Event
	messages message.Service
	query    agent.Runner
	nowFunc  func() time.Time
}

func NewService(st Store, llm provider.StreamingLLM, nowFunc func() time.Time) *Service {
	if nowFunc == nil {
		nowFunc = time.Now
	}

	appBus := bus.NewBus[Event](128)
	sessions := session.NewSessionService(st, nowFunc)
	messages := message.NewMessageService(st)
	query := agent.NewQueryLoop(messages, llm)

	svc := &Service{
		sessions: sessions,
		bus:      appBus,
		events:   appBus.Subscribe(context.Background()),
		nowFunc:  nowFunc,
		messages: messages,
		query:    query,
	}

	go svc.forwardSessionEvents(sessions.Events())
	go svc.forwardMessageEvents(messages.Events())
	go svc.forwardAgentEvents(query.Events())

	return svc
}

func (s *Service) Events() <-chan Event {
	return s.events
}

func (s *Service) EnsureActiveSession(ctx context.Context) (Session, error) {
	current, err := s.sessions.GetLast(ctx)
	if err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			return Session{}, err
		}

		current, err = s.sessions.Create(ctx, "New Session")
		if err != nil {
			return Session{}, err
		}
	}

	messages, err := s.messages.List(ctx, current.ID)
	if err != nil {
		return Session{}, err
	}

	s.bus.Publish(SessionReadyEvent{Session: toAppSession(current)})
	s.bus.Publish(ConversationHydratedEvent{
		SessionID: current.ID,
		Messages:  toAppHydratedMessages(messages),
	})

	return toAppSession(current), nil
}

func (s *Service) SendMessage(ctx context.Context, params SendMessageParams) (SendMessageResult, error) {
	result, err := s.query.RunQuery(ctx, agent.QueryParams{
		SessionID: params.SessionID,
		InputParts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: params.Prompt,
			},
		},
	})
	if err != nil {
		return SendMessageResult{
			User:      Message{ID: result.UserMessageID},
			Assistant: Message{ID: result.FinalAssistantMessageID},
		}, err
	}

	return SendMessageResult{
		User:      Message{ID: result.UserMessageID},
		Assistant: Message{ID: result.FinalAssistantMessageID},
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

func (s *Service) forwardSessionEvents(events <-chan session.Event) {
	for range events {
		// Session-specific app events can be introduced here as session workflows expand.
	}
}

func (s *Service) forwardAgentEvents(events <-chan agent.Event) {
	for range events {
		// Query lifecycle events are intentionally not remapped to app message events.
		// Message status changes are published by message.Service and forwarded separately.
	}
}

func toAppSession(session session.Session) Session {
	return Session{
		ID:           session.ID,
		Title:        session.Title,
		CreatedAt:    session.CreatedAt,
		UpdatedAt:    session.UpdatedAt,
		LastActiveAt: session.LastActiveAt,
	}
}

func toAppHydratedMessages(messages []message.Message) []Message {
	out := make([]Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, toAppMessage(msg))
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
