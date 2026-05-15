package ui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/lifecycle"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/session"
	"github.com/YaHeii/agentGo/internal/store"
)

func TestInitBootstrapsSessionAndLoadsHistory(t *testing.T) {
	t.Parallel()
	lifecycle.State = nil

	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) error {
		svc.events <- app.BaseEvent{
			T: app.EventSession,
			Payload: session.SessionEvent{
				Status: session.StatusRestored,
				Session: &store.Session{
					ID:    "session-1",
					Title: "demo",
				},
			},
		}
		svc.events <- app.BaseEvent{
			T: app.EventAgent,
			Payload: agent.QueryEvent{
				Status: agent.QueryStatusDelta,
				State: agent.LoopState{
					Messages: []message.Message{
						messageRecord("u1", message.KindUser, "hello"),
						messageRecord("a1", message.KindAssistant, "world"),
					},
					Transition: "history_loaded",
				},
			},
		}
		return nil
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
	if lifecycle.GetState().PermissionLevel != lifecycle.SafeLevel {
		t.Fatalf("expected safe permission level, got %v", lifecycle.GetState().PermissionLevel)
	}
}

func TestEnterDispatchesSendAndAppliesQueryEvents(t *testing.T) {
	t.Parallel()

	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) error {
		svc.events <- app.BaseEvent{
			T: app.EventSession,
			Payload: session.SessionEvent{
				Status: session.StatusRestored,
				Session: &store.Session{
					ID:    "session-1",
					Title: "demo",
				},
			},
		}
		return nil
	}
	svc.sendMessageFn = func(_ context.Context, sessionID string, prompt string) error {
		svc.events <- app.BaseEvent{
			T: app.EventAgent,
			Payload: agent.QueryEvent{
				Status: agent.QueryStatusStarted,
				State: agent.LoopState{
					Messages: []message.Message{
						messageRecord("user-1", message.KindUser, prompt),
					},
					Transition: "user_message_created",
				},
			},
		}
		svc.events <- app.BaseEvent{
			T: app.EventAgent,
			Payload: agent.QueryEvent{
				Status: agent.QueryStatusDelta,
				State: agent.LoopState{
					Messages: []message.Message{
						messageRecord("user-1", message.KindUser, prompt),
						messageRecord("assistant-1", message.KindAssistant, "hel"),
					},
					Transition: "assistant_delta_received",
				},
			},
		}
		svc.events <- app.BaseEvent{
			T: app.EventAgent,
			Payload: agent.QueryEvent{
				Status: agent.QueryStatusCompleted,
				State: agent.LoopState{
					Messages: []message.Message{
						messageRecord("user-1", message.KindUser, prompt),
						messageRecord("assistant-1", message.KindAssistant, "hello"),
					},
					Transition: "assistant_completed",
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

	for i := 0; i < 3; i++ {
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
		svc.events <- app.BaseEvent{
			T: app.EventSession,
			Payload: session.SessionEvent{
				Status: session.StatusRestored,
				Session: &store.Session{
					ID:    "session-1",
					Title: "demo",
				},
			},
		}
		return nil
	}
	svc.sendMessageFn = func(_ context.Context, sessionID string, prompt string) error {
		svc.events <- app.BaseEvent{
			T: app.EventAgent,
			Payload: agent.QueryEvent{
				Status: agent.QueryStatusDelta,
				State: agent.LoopState{
					Messages: []message.Message{
						messageRecord("user-1", message.KindUser, prompt),
						messageRecord("assistant-1", message.KindAssistant, "par"),
					},
					Transition: "stream_failed",
				},
				Err: errors.New("stream failed"),
			},
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

	updated, _ = model.Update(listenCmd())
	model = updated.(rootModel)

	if textContent(model.messages[1].Parts) != "par" {
		t.Fatalf("expected partial reply, got %q", textContent(model.messages[1].Parts))
	}

	updated, _ = model.Update(sendMessageDoneMsg{err: errors.New("stream failed")})
	model = updated.(rootModel)
	if model.errMessage == "" {
		t.Fatal("expected error message")
	}
}

func TestQueryFailureEventStopsLoadingAndShowsError(t *testing.T) {
	t.Parallel()

	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) error {
		svc.events <- app.BaseEvent{
			T: app.EventSession,
			Payload: session.SessionEvent{
				Status: session.StatusRestored,
				Session: &store.Session{
					ID:    "session-1",
					Title: "demo",
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

	model.loading = true
	svc.events <- app.BaseEvent{
		T: app.EventAgent,
		Payload: agent.QueryEvent{
			Status: agent.QueryStatusFailed,
			State: agent.LoopState{
				Transition: "stream_failed",
			},
			Err: errors.New("query failed"),
		},
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
		svc.events <- app.BaseEvent{
			T: app.EventSession,
			Payload: session.SessionEvent{
				Status: session.StatusRestored,
				Session: &store.Session{
					ID:    "session-1",
					Title: "demo",
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

	model.loading = true
	svc.events <- app.BaseEvent{
		T: app.EventAgent,
		Payload: agent.QueryEvent{
			Status: agent.QueryStatusCompleted,
			State: agent.LoopState{
				Transition: "assistant_completed",
			},
		},
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

func (s *stubChatService) InitializePermissionLevel(_ context.Context) error {
	return nil
}

func enterKey() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
}

func messageRecord(id string, kind message.Kind, text string) message.Message {
	return message.Message{
		ID:        id,
		SessionID: "session-1",
		Kind:      kind,
		Parts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: text,
			},
		},
	}
}
