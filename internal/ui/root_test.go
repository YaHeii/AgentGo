package ui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/app"
)

func TestInitBootstrapsSessionAndLoadsHistory(t *testing.T) {
	t.Parallel()

	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) (app.Session, error) {
		svc.events <- app.SessionReadyEvent{
			Session: app.Session{ID: "session-1", Title: "demo"},
		}
		svc.events <- app.ConversationHydratedEvent{
			SessionID: "session-1",
			Messages: []app.Message{
				{ID: "u1", SessionID: "session-1", Role: roleUser, Content: "hello"},
				{ID: "a1", SessionID: "session-1", Role: roleAssistant, Content: "world"},
			},
		}
		return app.Session{ID: "session-1", Title: "demo"}, nil
	}

	model := NewRootModel(svc)

	initMsg := model.Init()()
	updated, listenCmd := model.Update(initMsg)
	next := updated.(rootModel)

	updated, listenCmd = next.Update(listenCmd())
	next = updated.(rootModel)

	updated, _ = next.Update(listenCmd())
	next = updated.(rootModel)

	if next.sessionID != "session-1" {
		t.Fatalf("expected active session, got %q", next.sessionID)
	}
	if len(next.messages) != 2 {
		t.Fatalf("expected 2 hydrated messages, got %d", len(next.messages))
	}
}

func TestEnterDispatchesSendAndAppliesStreamEvents(t *testing.T) {
	t.Parallel()

	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) (app.Session, error) {
		svc.events <- app.SessionReadyEvent{
			Session: app.Session{ID: "session-1", Title: "demo"},
		}
		svc.events <- app.ConversationHydratedEvent{
			SessionID: "session-1",
			Messages:  nil,
		}
		return app.Session{ID: "session-1", Title: "demo"}, nil
	}
	svc.sendMessageFn = func(_ context.Context, params app.SendMessageParams) (app.SendMessageResult, error) {
		svc.events <- app.MessageCreatedEvent{
			Message: app.Message{ID: "user-1", SessionID: params.SessionID, Role: roleUser, Content: params.Prompt},
		}
		svc.events <- app.MessageCreatedEvent{
			Message: app.Message{ID: "assistant-1", SessionID: params.SessionID, Role: roleAssistant, Content: "", Status: app.MessageStatusStreaming},
		}
		svc.events <- app.MessageDeltaEvent{
			Message: app.Message{ID: "assistant-1", SessionID: params.SessionID, Role: roleAssistant, Content: "hel", Status: app.MessageStatusStreaming},
			Delta:   "hel",
		}
		svc.events <- app.MessageCompletedEvent{
			Message: app.Message{ID: "assistant-1", SessionID: params.SessionID, Role: roleAssistant, Content: "hello", Status: app.MessageStatusComplete},
		}
		return app.SendMessageResult{}, nil
	}

	model := NewRootModel(svc)

	bootstrapMsg := model.Init()()
	updated, listenCmd := model.Update(bootstrapMsg)
	model = updated.(rootModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(rootModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(rootModel)

	model.input = "hello"

	updated, sendCmd := model.Update(enterKey())
	model = updated.(rootModel)

	if !model.loading {
		t.Fatal("expected loading state after enter")
	}

	_ = sendCmd()

	for i := 0; i < 4; i++ {
		updated, listenCmd = model.Update(listenCmd())
		model = updated.(rootModel)
	}

	if model.loading {
		t.Fatal("expected loading to stop after completion event")
	}
	if len(model.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(model.messages))
	}
	if model.messages[1].Content != "hello" {
		t.Fatalf("expected streamed assistant content, got %q", model.messages[1].Content)
	}
}

func TestStreamFailureShowsErrorAndKeepsPartialAssistantMessage(t *testing.T) {
	t.Parallel()

	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) (app.Session, error) {
		svc.events <- app.SessionReadyEvent{
			Session: app.Session{ID: "session-1", Title: "demo"},
		}
		svc.events <- app.ConversationHydratedEvent{
			SessionID: "session-1",
			Messages:  nil,
		}
		return app.Session{ID: "session-1", Title: "demo"}, nil
	}
	svc.sendMessageFn = func(_ context.Context, params app.SendMessageParams) (app.SendMessageResult, error) {
		svc.events <- app.MessageCreatedEvent{
			Message: app.Message{ID: "user-1", SessionID: params.SessionID, Role: roleUser, Content: params.Prompt},
		}
		svc.events <- app.MessageCreatedEvent{
			Message: app.Message{ID: "assistant-1", SessionID: params.SessionID, Role: roleAssistant, Content: "", Status: app.MessageStatusStreaming},
		}
		svc.events <- app.MessageDeltaEvent{
			Message: app.Message{ID: "assistant-1", SessionID: params.SessionID, Role: roleAssistant, Content: "par", Status: app.MessageStatusStreaming},
			Delta:   "par",
		}
		svc.events <- app.MessageFailedEvent{
			Message: app.Message{ID: "assistant-1", SessionID: params.SessionID, Role: roleAssistant, Content: "par", Status: app.MessageStatusFailed},
			Err:     errors.New("stream failed"),
		}
		return app.SendMessageResult{}, errors.New("stream failed")
	}

	model := NewRootModel(svc)

	bootstrapMsg := model.Init()()
	updated, listenCmd := model.Update(bootstrapMsg)
	model = updated.(rootModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(rootModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(rootModel)

	model.input = "hello"
	updated, sendCmd := model.Update(enterKey())
	model = updated.(rootModel)
	_ = sendCmd()

	for i := 0; i < 4; i++ {
		updated, listenCmd = model.Update(listenCmd())
		model = updated.(rootModel)
	}

	if model.loading {
		t.Fatal("expected loading to stop after failure event")
	}
	if model.errMessage == "" {
		t.Fatal("expected error message")
	}
	if model.messages[1].Content != "par" {
		t.Fatalf("expected partial reply, got %q", model.messages[1].Content)
	}
}

func TestQueryFailureEventStopsLoadingAndShowsError(t *testing.T) {
	t.Parallel()

	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) (app.Session, error) {
		svc.events <- app.SessionReadyEvent{
			Session: app.Session{ID: "session-1", Title: "demo"},
		}
		svc.events <- app.ConversationHydratedEvent{
			SessionID: "session-1",
			Messages:  nil,
		}
		return app.Session{ID: "session-1", Title: "demo"}, nil
	}

	model := NewRootModel(svc)

	bootstrapMsg := model.Init()()
	updated, listenCmd := model.Update(bootstrapMsg)
	model = updated.(rootModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(rootModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(rootModel)

	model.loading = true
	svc.events <- app.QueryFailedEvent{
		SessionID:          "session-1",
		UserMessageID:      "user-1",
		AssistantMessageID: "assistant-1",
		Err:                errors.New("query failed"),
	}

	updated, listenCmd = model.Update(listenCmd())
	model = updated.(rootModel)

	if model.loading {
		t.Fatal("expected loading to stop after query failure")
	}
	if model.errMessage != "query failed" {
		t.Fatalf("expected query failure error, got %q", model.errMessage)
	}
}

func TestQueryCompletedEventStopsLoading(t *testing.T) {
	t.Parallel()

	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) (app.Session, error) {
		svc.events <- app.SessionReadyEvent{
			Session: app.Session{ID: "session-1", Title: "demo"},
		}
		svc.events <- app.ConversationHydratedEvent{
			SessionID: "session-1",
			Messages:  nil,
		}
		return app.Session{ID: "session-1", Title: "demo"}, nil
	}

	model := NewRootModel(svc)

	bootstrapMsg := model.Init()()
	updated, listenCmd := model.Update(bootstrapMsg)
	model = updated.(rootModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(rootModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(rootModel)

	model.loading = true
	svc.events <- app.QueryCompletedEvent{
		SessionID:          "session-1",
		UserMessageID:      "user-1",
		AssistantMessageID: "assistant-1",
	}

	updated, _ = model.Update(listenCmd())
	model = updated.(rootModel)

	if model.loading {
		t.Fatal("expected loading to stop after query completion")
	}
}

type stubChatService struct {
	events          chan app.Event
	ensureSessionFn func(ctx context.Context) (app.Session, error)
	sendMessageFn   func(ctx context.Context, params app.SendMessageParams) (app.SendMessageResult, error)
}

func newStubChatService() *stubChatService {
	return &stubChatService{
		events: make(chan app.Event, 16),
	}
}

func (s *stubChatService) EnsureActiveSession(ctx context.Context) (app.Session, error) {
	if s.ensureSessionFn == nil {
		return app.Session{}, nil
	}
	return s.ensureSessionFn(ctx)
}

func (s *stubChatService) SendMessage(ctx context.Context, params app.SendMessageParams) (app.SendMessageResult, error) {
	if s.sendMessageFn == nil {
		return app.SendMessageResult{}, nil
	}
	return s.sendMessageFn(ctx, params)
}

func (s *stubChatService) Events() <-chan app.Event {
	return s.events
}

func enterKey() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
}
