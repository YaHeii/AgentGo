package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/YaHeii/agentGo/internal/utils"
)

func TestValidateProviderConfigRequiresAPIKeyAndModel(t *testing.T) {
	t.Parallel()

	_, err := ProviderConfigFromAppConfig(utils.Config{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "API_KEY") {
		t.Fatalf("expected API_KEY error, got %v", err)
	}

	_, err = ProviderConfigFromAppConfig(utils.Config{APIKey: "test-key"})
	if err == nil {
		t.Fatal("expected missing MODEL error")
	}
	if !strings.Contains(err.Error(), "MODEL") {
		t.Fatalf("expected MODEL error, got %v", err)
	}
}

func TestInitBootstrapsSessionAndLoadsHistory(t *testing.T) {
	t.Parallel()

	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) (store.Session, error) {
		svc.events <- app.SessionReadyEvent{
			Session: store.Session{ID: "session-1", Title: "demo"},
		}
		svc.events <- app.ConversationHydratedEvent{
			SessionID: "session-1",
			Messages: []store.Message{
				{ID: "u1", SessionID: "session-1", Role: roleUser, Content: "hello"},
				{ID: "a1", SessionID: "session-1", Role: roleAssistant, Content: "world"},
			},
		}
		return store.Session{ID: "session-1", Title: "demo"}, nil
	}

	model := NewChatModel(svc)

	initMsg := model.Init()()
	updated, listenCmd := model.Update(initMsg)
	next := updated.(chatModel)

	updated, listenCmd = next.Update(listenCmd())
	next = updated.(chatModel)

	updated, _ = next.Update(listenCmd())
	next = updated.(chatModel)

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
	svc.ensureSessionFn = func(_ context.Context) (store.Session, error) {
		svc.events <- app.SessionReadyEvent{
			Session: store.Session{ID: "session-1", Title: "demo"},
		}
		svc.events <- app.ConversationHydratedEvent{
			SessionID: "session-1",
			Messages:  nil,
		}
		return store.Session{ID: "session-1", Title: "demo"}, nil
	}
	svc.sendMessageFn = func(_ context.Context, params app.SendMessageParams) (app.SendMessageResult, error) {
		svc.events <- app.MessageCreatedEvent{
			Message: store.Message{ID: "user-1", SessionID: params.SessionID, Role: roleUser, Content: params.Prompt},
		}
		svc.events <- app.MessageCreatedEvent{
			Message: store.Message{ID: "assistant-1", SessionID: params.SessionID, Role: roleAssistant, Content: "", Status: store.MessageStatusStreaming},
		}
		svc.events <- app.MessageDeltaEvent{
			Message: store.Message{ID: "assistant-1", SessionID: params.SessionID, Role: roleAssistant, Content: "hel", Status: store.MessageStatusStreaming},
			Delta:   "hel",
		}
		svc.events <- app.MessageCompletedEvent{
			Message: store.Message{ID: "assistant-1", SessionID: params.SessionID, Role: roleAssistant, Content: "hello", Status: store.MessageStatusComplete},
		}
		return app.SendMessageResult{}, nil
	}

	model := NewChatModel(svc)

	bootstrapMsg := model.Init()()
	updated, listenCmd := model.Update(bootstrapMsg)
	model = updated.(chatModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(chatModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(chatModel)

	model.input = "hello"

	updated, sendCmd := model.Update(enterKey())
	model = updated.(chatModel)

	if !model.loading {
		t.Fatal("expected loading state after enter")
	}

	_ = sendCmd()

	for i := 0; i < 4; i++ {
		updated, listenCmd = model.Update(listenCmd())
		model = updated.(chatModel)
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
	svc.ensureSessionFn = func(_ context.Context) (store.Session, error) {
		svc.events <- app.SessionReadyEvent{
			Session: store.Session{ID: "session-1", Title: "demo"},
		}
		svc.events <- app.ConversationHydratedEvent{
			SessionID: "session-1",
			Messages:  nil,
		}
		return store.Session{ID: "session-1", Title: "demo"}, nil
	}
	svc.sendMessageFn = func(_ context.Context, params app.SendMessageParams) (app.SendMessageResult, error) {
		svc.events <- app.MessageCreatedEvent{
			Message: store.Message{ID: "user-1", SessionID: params.SessionID, Role: roleUser, Content: params.Prompt},
		}
		svc.events <- app.MessageCreatedEvent{
			Message: store.Message{ID: "assistant-1", SessionID: params.SessionID, Role: roleAssistant, Content: "", Status: store.MessageStatusStreaming},
		}
		svc.events <- app.MessageDeltaEvent{
			Message: store.Message{ID: "assistant-1", SessionID: params.SessionID, Role: roleAssistant, Content: "par", Status: store.MessageStatusStreaming},
			Delta:   "par",
		}
		svc.events <- app.MessageFailedEvent{
			Message: store.Message{ID: "assistant-1", SessionID: params.SessionID, Role: roleAssistant, Content: "par", Status: store.MessageStatusFailed},
			Err:     errors.New("stream failed"),
		}
		return app.SendMessageResult{}, errors.New("stream failed")
	}

	model := NewChatModel(svc)

	bootstrapMsg := model.Init()()
	updated, listenCmd := model.Update(bootstrapMsg)
	model = updated.(chatModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(chatModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(chatModel)

	model.input = "hello"
	updated, sendCmd := model.Update(enterKey())
	model = updated.(chatModel)
	_ = sendCmd()

	for i := 0; i < 4; i++ {
		updated, listenCmd = model.Update(listenCmd())
		model = updated.(chatModel)
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

type stubChatService struct {
	events          chan app.Event
	ensureSessionFn func(ctx context.Context) (store.Session, error)
	sendMessageFn   func(ctx context.Context, params app.SendMessageParams) (app.SendMessageResult, error)
}

func newStubChatService() *stubChatService {
	return &stubChatService{
		events: make(chan app.Event, 16),
	}
}

func (s *stubChatService) EnsureActiveSession(ctx context.Context) (store.Session, error) {
	if s.ensureSessionFn == nil {
		return store.Session{}, nil
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
