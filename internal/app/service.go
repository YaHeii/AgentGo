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

type appStore interface {
	sessionPersistence
	messageStore
}

type sessionPersistence interface {
	CreateSession(ctx context.Context, params store.CreateSessionParams) (store.Session, error)
	ListSessions(ctx context.Context) ([]store.Session, error)
	GetSession(ctx context.Context, id string) (store.Session, error)
	UpdateSession(ctx context.Context, params store.UpdateSessionParams) (store.Session, error)
	DeleteSession(ctx context.Context, id string) error
}

type messageStore interface {
	CreateMessage(ctx context.Context, params store.CreateMessageParams) (store.Message, error)
	ListMessages(ctx context.Context, sessionID string) ([]store.Message, error)
}

type messageService interface {
	Create(ctx context.Context, sessionID string, params message.CreateMessageParams) (message.Message, error)
	Update(ctx context.Context, message message.Message) error
	Get(ctx context.Context, id string) (message.Message, error)
	List(ctx context.Context, sessionID string) ([]message.Message, error)
	ListUserMessages(ctx context.Context, sessionID string) ([]message.Message, error)
	ListAllUserMessages(ctx context.Context) ([]message.Message, error)
	Delete(ctx context.Context, id string) error
	DeleteSessionMessages(ctx context.Context, sessionID string) error
	Events() <-chan message.Event
}

type sessionService interface {
	Create(ctx context.Context, title string) (session.Session, error)
	Get(ctx context.Context, id string) (session.Session, error)
	GetLast(ctx context.Context) (session.Session, error)
	List(ctx context.Context) ([]session.Session, error)
	Rename(ctx context.Context, id string, title string) (session.Session, error)
	Delete(ctx context.Context, id string) error
	Events() <-chan session.Event
}

type queryRunner interface {
	RunQuery(ctx context.Context, params agent.QueryParams) (agent.QueryResult, error)
	Events() <-chan agent.Event
}

type Service struct {
	sessions sessionService
	bus      bus.Bus[Event]
	events   <-chan Event
	messages messageService
	query    queryRunner
	nowFunc  func() time.Time
}

func NewService(st appStore, llm provider.StreamingLLM, nowFunc func() time.Time) *Service {
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
			User:             Message{ID: result.UserMessageID},
			Assistant:        Message{ID: result.FinalAssistantMessageID},
			FinishReason:     toAppFinishReason(result.FinishReason),
			PendingToolCalls: toAppToolCalls(result.PendingToolCalls),
		}, err
	}

	return SendMessageResult{
		User:             Message{ID: result.UserMessageID},
		Assistant:        Message{ID: result.FinalAssistantMessageID},
		FinishReason:     toAppFinishReason(result.FinishReason),
		PendingToolCalls: toAppToolCalls(result.PendingToolCalls),
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
	for evt := range events {
		switch event := evt.(type) {
		case agent.QueryCompletedEvent:
			s.bus.Publish(QueryCompletedEvent{
				SessionID:          event.SessionID,
				UserMessageID:      event.UserMessageID,
				AssistantMessageID: event.AssistantMessageID,
			})
		case agent.QueryFailedEvent:
			s.bus.Publish(QueryFailedEvent{
				SessionID:          event.SessionID,
				UserMessageID:      event.UserMessageID,
				AssistantMessageID: event.AssistantMessageID,
				Err:                event.Err,
			})
		}
	}
}

func toAppSession(session session.Session) Session {
	return Session{
		ID:        session.ID,
		Title:     session.Title,
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.UpdatedAt,
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

func toAppFinishReason(reason agent.FinishReason) QueryFinishReason {
	switch reason {
	case agent.FinishReasonAwaitingToolExecution:
		return QueryFinishReasonAwaitingToolExecution
	case agent.FinishReasonCancelled:
		return QueryFinishReasonCancelled
	case agent.FinishReasonFailed:
		return QueryFinishReasonFailed
	default:
		return QueryFinishReasonCompleted
	}
}

func toAppToolCalls(calls []provider.ToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}

	out := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, ToolCall{
			Index:     call.Index,
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return out
}
