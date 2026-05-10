package ui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/session"
	"github.com/YaHeii/agentGo/internal/store"
)

func TestInitBootstrapsSessionAndLoadsHistory(t *testing.T) {
	t.Parallel()

	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) error {
		svc.events <- session.SessionRestoredEvent{
			Session: store.Session{ID: "session-1", Title: "demo"},
			Messages: []message.Message{
				messageRecord("u1", message.KindUser, "hello"),
				messageRecord("a1", message.KindAssistant, "world"),
			},
		}
		return nil
	}

	model := NewRootModel(svc)

	initMsg := model.Init()()
	updated, listenCmd := model.Update(initMsg)
	next := updated.(rootModel)

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
	svc.ensureSessionFn = func(_ context.Context) error {
		svc.events <- session.SessionRestoredEvent{
			Session:  store.Session{ID: "session-1", Title: "demo"},
			Messages: nil,
		}
		return nil
	}
	svc.sendMessageFn = func(_ context.Context, sessionID string, prompt string) error {
		svc.events <- message.MessageCreatedEvent{
			Message: message.Message{
				ID:        "user-1",
				SessionID: sessionID,
				Kind:      message.KindUser,
				Status:    message.StatusComplete,
				Parts: []message.Part{
					{Type: message.PartTypeText, Text: prompt},
				},
			},
		}
		svc.events <- message.MessageCreatedEvent{
			Message: message.Message{
				ID:        "assistant-1",
				SessionID: sessionID,
				Kind:      message.KindAssistant,
				Status:    message.StatusStreaming,
				Parts: []message.Part{
					{Type: message.PartTypeText, Text: ""},
				},
			},
		}
		svc.events <- message.MessageDeltaEvent{
			Message: message.Message{
				ID:        "assistant-1",
				SessionID: sessionID,
				Kind:      message.KindAssistant,
				Status:    message.StatusStreaming,
				Parts: []message.Part{
					{Type: message.PartTypeText, Text: "hel"},
				},
			},
			Delta: "hel",
		}
		svc.events <- message.MessageCompletedEvent{
			Message: message.Message{
				ID:        "assistant-1",
				SessionID: sessionID,
				Kind:      message.KindAssistant,
				Status:    message.StatusComplete,
				Parts: []message.Part{
					{Type: message.PartTypeText, Text: "hello"},
				},
			},
		}
		return nil
	}

	model := NewRootModel(svc)

	bootstrapMsg := model.Init()()
	updated, listenCmd := model.Update(bootstrapMsg)
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
	if textContent(model.messages[1].Parts) != "hello" {
		t.Fatalf("expected streamed assistant content, got %q", textContent(model.messages[1].Parts))
	}
}

func TestStreamFailureShowsErrorAndKeepsPartialAssistantMessage(t *testing.T) {
	t.Parallel()

	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) error {
		svc.events <- session.SessionRestoredEvent{
			Session:  store.Session{ID: "session-1", Title: "demo"},
			Messages: nil,
		}
		return nil
	}
	svc.sendMessageFn = func(_ context.Context, sessionID string, prompt string) error {
		svc.events <- message.MessageCreatedEvent{
			Message: message.Message{
				ID:        "user-1",
				SessionID: sessionID,
				Kind:      message.KindUser,
				Status:    message.StatusComplete,
				Parts: []message.Part{
					{Type: message.PartTypeText, Text: prompt},
				},
			},
		}
		svc.events <- message.MessageCreatedEvent{
			Message: message.Message{
				ID:        "assistant-1",
				SessionID: sessionID,
				Kind:      message.KindAssistant,
				Status:    message.StatusStreaming,
				Parts: []message.Part{
					{Type: message.PartTypeText, Text: ""},
				},
			},
		}
		svc.events <- message.MessageDeltaEvent{
			Message: message.Message{
				ID:        "assistant-1",
				SessionID: sessionID,
				Kind:      message.KindAssistant,
				Status:    message.StatusStreaming,
				Parts: []message.Part{
					{Type: message.PartTypeText, Text: "par"},
				},
			},
			Delta: "par",
		}
		svc.events <- message.MessageFailedEvent{
			Message: message.Message{
				ID:        "assistant-1",
				SessionID: sessionID,
				Kind:      message.KindAssistant,
				Status:    message.StatusFailed,
				Parts: []message.Part{
					{Type: message.PartTypeText, Text: "par"},
				},
			},
			Err: errors.New("stream failed"),
		}
		return errors.New("stream failed")
	}

	model := NewRootModel(svc)

	bootstrapMsg := model.Init()()
	updated, listenCmd := model.Update(bootstrapMsg)
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
	if textContent(model.messages[1].Parts) != "par" {
		t.Fatalf("expected partial reply, got %q", textContent(model.messages[1].Parts))
	}
}

func TestQueryFailureEventStopsLoadingAndShowsError(t *testing.T) {
	t.Parallel()

	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) error {
		svc.events <- session.SessionRestoredEvent{
			Session:  store.Session{ID: "session-1", Title: "demo"},
			Messages: nil,
		}
		return nil
	}

	model := NewRootModel(svc)

	bootstrapMsg := model.Init()()
	updated, listenCmd := model.Update(bootstrapMsg)
	model = updated.(rootModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(rootModel)

	model.loading = true
	svc.events <- agent.QueryFailedEvent{
		SessionID:          "session-1",
		UserMessageID:      "user-1",
		AssistantMessageID: "assistant-1",
		Err:                errors.New("query failed"),
	}

	updated, _ = model.Update(listenCmd())
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
	svc.ensureSessionFn = func(_ context.Context) error {
		svc.events <- session.SessionRestoredEvent{
			Session:  store.Session{ID: "session-1", Title: "demo"},
			Messages: nil,
		}
		return nil
	}

	model := NewRootModel(svc)

	bootstrapMsg := model.Init()()
	updated, listenCmd := model.Update(bootstrapMsg)
	model = updated.(rootModel)
	updated, listenCmd = model.Update(listenCmd())
	model = updated.(rootModel)

	model.loading = true
	svc.events <- agent.QueryCompletedEvent{
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
	ensureSessionFn func(ctx context.Context) error
	sendMessageFn   func(ctx context.Context, sessionID string, prompt string) error
}

func newStubChatService() *stubChatService {
	return &stubChatService{
		events: make(chan app.Event, 16),
	}
}

func (s *stubChatService) EnsureActiveSession(ctx context.Context) error {
	if s.ensureSessionFn == nil {
		return nil
	}
	return s.ensureSessionFn(ctx)
}

func (s *stubChatService) SendMessage(ctx context.Context, sessionID string, prompt string) error {
	if s.sendMessageFn == nil {
		return nil
	}
	return s.sendMessageFn(ctx, sessionID, prompt)
}

func (s *stubChatService) Events() <-chan app.Event {
	return s.events
}

func enterKey() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
}

func messageRecord(id string, kind message.Kind, text string) message.Message {
	return message.Message{
		ID:        id,
		SessionID: "session-1",
		Kind:      kind,
		Status:    message.StatusComplete,
		Parts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: text,
			},
		},
	}
}
