package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/lifecycle"
	message "github.com/YaHeii/agentGo/internal/message/contract"
	"github.com/YaHeii/agentGo/internal/session"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
)

func TestInitBootstrapsSessionAndLoadsHistory(t *testing.T) {
	lifecycle.State = nil

	svc := newStubChatService()
	svc.history = []message.Message{
		messageRecord("u1", message.KindUser, "hello"),
		messageRecord("a1", message.KindAssistant, "world"),
	}
	svc.ensureSessionFn = func(_ context.Context) error {
		svc.events <- app.BaseEvent{
			T: app.EventSession,
			Payload: session.SessionEvent{
				Status: session.StatusRestored,
				Session: &sessioncontract.Session{
					ID:    "session-1",
					Title: "demo",
				},
			},
		}
		return nil
	}

	model := NewRootModel(svc)

	initMsg := model.Init()()
	updated, listenCmd := model.Update(initMsg)
	next := updated.(rootModel)

	updated, historyCmd := next.Update(listenCmd())
	next = updated.(rootModel)
	updated, _ = next.Update(historyCmd())
	next = updated.(rootModel)

	if next.sessionID != "session-1" {
		t.Fatalf("expected active session, got %q", next.sessionID)
	}
	if len(next.messages) != 2 {
		t.Fatalf("expected 2 hydrated messages, got %d", len(next.messages))
	}
	if lifecycle.State == nil {
		t.Fatal("expected lifecycle state to be initialized")
	}
	if lifecycle.State.PermissionLevel != lifecycle.SafeLevel {
		t.Fatalf("expected safe permission level, got %v", lifecycle.State.PermissionLevel)
	}
}

func TestRootComposesChatComponents(t *testing.T) {
	model := NewRootModel(newStubChatService())

	if model.transcript.viewport.Width != defaultWidth {
		t.Fatalf("expected transcript viewport width %d, got %d", defaultWidth, model.transcript.viewport.Width)
	}
	if model.composer.value != "" {
		t.Fatalf("expected empty composer, got %q", model.composer.value)
	}
	if model.status.message != "" {
		t.Fatalf("expected empty status message, got %q", model.status.message)
	}

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Text: "h"}))
	next := updated.(rootModel)

	if next.composer.value != "h" {
		t.Fatalf("expected composer to hold input, got %q", next.composer.value)
	}
}

func TestRootLoadsHistoryAfterSessionRestore(t *testing.T) {
	svc := newStubChatService()
	svc.history = []message.Message{
		messageRecord("history-user", message.KindUser, "old question"),
		messageRecord("history-assistant", message.KindAssistant, "old answer"),
	}

	model := NewRootModel(svc)
	updated, listenCmd := model.Update(bootstrapDoneMsg{})
	model = updated.(rootModel)

	svc.events <- app.BaseEvent{
		T: app.EventSession,
		Payload: session.SessionEvent{
			Status: session.StatusRestored,
			Session: &sessioncontract.Session{
				ID:    "session-1",
				Title: "demo",
			},
		},
	}

	updated, historyCmd := model.Update(listenCmd())
	model = updated.(rootModel)
	if model.sessionID != "session-1" {
		t.Fatalf("expected restored session, got %q", model.sessionID)
	}

	updated, _ = model.Update(historyCmd())
	model = updated.(rootModel)

	if svc.lastHistorySessionID != "session-1" {
		t.Fatalf("expected history load for session-1, got %q", svc.lastHistorySessionID)
	}
	if len(model.messages) != 2 {
		t.Fatalf("expected 2 history messages, got %d", len(model.messages))
	}
	if len(model.transcript.items) != 2 {
		t.Fatalf("expected 2 transcript items, got %d", len(model.transcript.items))
	}
}

func TestTranscriptBuildsCollapsibleItemsForStructuredParts(t *testing.T) {
	messages := []message.Message{
		{
			ID:        "assistant-1",
			SessionID: "session-1",
			Kind:      message.KindAssistant,
			Parts: []message.Part{
				{Type: message.PartTypeText, Text: "final answer"},
				{
					Type: message.PartTypeThinking,
					Thinking: &message.ThinkingPart{
						Content: "expanded reasoning",
						Summary: "brief reasoning",
					},
				},
				{
					Type: message.PartTypeToolCall,
					ToolCall: &message.ToolCallPart{
						ID:    "call-1",
						Name:  "grep",
						Input: "{\"pattern\":\"agentGo\"}",
					},
				},
				{
					Type: message.PartTypeToolResult,
					ToolResult: &message.ToolResultPart{
						ToolCallID: "call-1",
						Content:    "{\"matches\":1}",
					},
				},
			},
		},
		{
			ID:        "system-1",
			SessionID: "session-1",
			Kind:      message.KindSystem,
			Parts: []message.Part{
				{Type: message.PartTypeText, Text: "provider unavailable"},
			},
			System: &message.SystemPayload{
				Subtype: "provider_error",
				Level:   "error",
			},
			Progress: &message.ProgressPayload{
				Stage:   "tool_call",
				Current: 1,
				Total:   3,
			},
		},
	}

	items := buildTranscriptItems(messages, nil, nil)

	if len(items) != 6 {
		t.Fatalf("expected 6 transcript items, got %d", len(items))
	}
	if items[0].Summary != "ai: final answer" {
		t.Fatalf("expected assistant text summary, got %q", items[0].Summary)
	}
	if items[1].Summary != "thinking" {
		t.Fatalf("expected thinking summary, got %q", items[1].Summary)
	}
	if items[1].Body != "expanded reasoning" {
		t.Fatalf("expected thinking body, got %q", items[1].Body)
	}
	if items[2].Summary != "tool call: grep" {
		t.Fatalf("expected tool call summary, got %q", items[2].Summary)
	}
	if items[3].Summary != "tool result: call-1" {
		t.Fatalf("expected tool result summary, got %q", items[3].Summary)
	}
	if items[4].Summary != "system: provider_error" {
		t.Fatalf("expected system summary, got %q", items[4].Summary)
	}
	if items[5].Summary != "progress: tool_call 1/3" {
		t.Fatalf("expected progress summary, got %q", items[5].Summary)
	}
}

func TestTranscriptTogglesSelectedItemExpansion(t *testing.T) {
	items := []transcriptItem{
		{ID: "assistant-1:0:text", Summary: "ai: hello", Body: "hello"},
		{ID: "assistant-1:1:thinking", Summary: "thinking", Body: "expanded reasoning"},
	}

	model := newTranscriptModel(defaultWidth, defaultHeight).setItems(items)
	model = model.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	if model.selected != 1 {
		t.Fatalf("expected selected item 1, got %d", model.selected)
	}

	model = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !model.expanded["assistant-1:1:thinking"] {
		t.Fatal("expected selected item to be expanded")
	}
	if !strings.Contains(model.View(), "expanded reasoning") {
		t.Fatalf("expected expanded body in transcript view, got %q", model.View())
	}

	model = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	if model.expanded["assistant-1:1:thinking"] {
		t.Fatal("expected selected item to collapse after space")
	}
}

func TestMarkdownRendersOnlyOnAgentTerminalEvent(t *testing.T) {
	renderer := &stubMarkdownRenderer{rendered: "rendered markdown"}
	model := NewRootModel(newStubChatService())
	model.renderer = renderer

	deltaEvent := appEventMsg{
		event: app.BaseEvent{
			T: app.EventAgent,
			Payload: agent.QueryEvent{
				Status: agent.QueryStatusDelta,
				State: agent.LoopState{
					Messages: []message.Message{
						messageRecord("assistant-1", message.KindAssistant, "**draft**"),
					},
				},
			},
		},
	}

	updated, _ := model.Update(deltaEvent)
	model = updated.(rootModel)

	if renderer.calls != 0 {
		t.Fatalf("expected no markdown render during delta, got %d", renderer.calls)
	}
	if model.markdown["assistant-1"] != "" {
		t.Fatalf("expected no markdown cache during delta, got %q", model.markdown["assistant-1"])
	}
	if model.transcript.items[0].Body != "**draft**" {
		t.Fatalf("expected raw body during delta, got %q", model.transcript.items[0].Body)
	}

	completedEvent := appEventMsg{
		event: app.BaseEvent{
			T: app.EventAgent,
			Payload: agent.QueryEvent{
				Status: agent.QueryStatusCompleted,
				State: agent.LoopState{
					Messages: []message.Message{
						messageRecord("assistant-1", message.KindAssistant, "**final**"),
					},
				},
			},
		},
	}

	updated, _ = model.Update(completedEvent)
	model = updated.(rootModel)

	if renderer.calls != 1 {
		t.Fatalf("expected one markdown render on completion, got %d", renderer.calls)
	}
	if model.markdown["assistant-1"] != "rendered markdown" {
		t.Fatalf("expected cached markdown, got %q", model.markdown["assistant-1"])
	}
	if model.transcript.items[0].Body != "rendered markdown" {
		t.Fatalf("expected rendered body after completion, got %q", model.transcript.items[0].Body)
	}
}

func TestRootStateMachineKeepsNetworkIOInCommands(t *testing.T) {
	svc := newStubChatService()
	model := NewRootModel(svc)
	model.sessionID = "session-1"
	model.composer.value = "hello"

	updated, cmd := model.Update(enterKey())
	model = updated.(rootModel)

	if svc.lastPrompt != "" {
		t.Fatalf("expected no RunQuery call during Update, got %q", svc.lastPrompt)
	}
	if cmd == nil {
		t.Fatal("expected runQueryCmd")
	}
	if !model.loading {
		t.Fatal("expected loading to start immediately after enter")
	}

	_ = cmd()

	if svc.lastPrompt != "hello" {
		t.Fatalf("expected RunQuery in command execution, got %q", svc.lastPrompt)
	}
}

func TestViewIsPure(t *testing.T) {
	renderer := &stubMarkdownRenderer{rendered: "rendered markdown"}
	model := NewRootModel(newStubChatService())
	model.renderer = renderer
	model.markdown["assistant-1"] = "cached markdown"
	model.transcript = model.transcript.setItems([]transcriptItem{
		{ID: "assistant-1:0:text", Summary: "ai: hello", Body: "cached markdown"},
		{ID: "assistant-1:1:thinking", Summary: "thinking", Body: "reasoning"},
	})
	model.transcript.selected = 1
	model.transcript.expanded["assistant-1:1:thinking"] = true
	model.status = model.status.setMessage("assistant is thinking...")

	beforeCalls := renderer.calls
	beforeSelected := model.transcript.selected
	beforeExpanded := model.transcript.expanded["assistant-1:1:thinking"]
	beforeMarkdown := model.markdown["assistant-1"]

	view := model.View()

	if view.Content == "" {
		t.Fatal("expected non-empty view")
	}
	if renderer.calls != beforeCalls {
		t.Fatalf("expected View to not render markdown, got %d calls", renderer.calls-beforeCalls)
	}
	if model.transcript.selected != beforeSelected {
		t.Fatalf("expected selected item unchanged, got %d", model.transcript.selected)
	}
	if model.transcript.expanded["assistant-1:1:thinking"] != beforeExpanded {
		t.Fatal("expected expanded state unchanged")
	}
	if model.markdown["assistant-1"] != beforeMarkdown {
		t.Fatalf("expected markdown cache unchanged, got %q", model.markdown["assistant-1"])
	}
}

func TestEnterRunsQueryAndAppliesMultiTurnEvents(t *testing.T) {
	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) error {
		svc.events <- app.BaseEvent{
			T: app.EventSession,
			Payload: session.SessionEvent{
				Status: session.StatusRestored,
				Session: &sessioncontract.Session{
					ID:    "session-1",
					Title: "demo",
				},
			},
		}
		return nil
	}
	svc.runQueryFn = func(_ context.Context, sessionID string, prompt string) error {
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
						messageRecord("assistant-1", message.KindAssistant, ""),
					},
					Transition: "awaiting_tool_execution",
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
						messageRecord("assistant-1", message.KindAssistant, ""),
						messageRecord("tool-1", message.KindSystem, `{"matches":["root_test.go"]}`),
					},
					Transition: "tool_results_recorded",
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
						messageRecord("assistant-1", message.KindAssistant, ""),
						messageRecord("tool-1", message.KindSystem, `{"matches":["root_test.go"]}`),
						messageRecord("assistant-2", message.KindAssistant, "hello"),
					},
					Transition: "assistant_completed",
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
						messageRecord("assistant-1", message.KindAssistant, ""),
						messageRecord("tool-1", message.KindSystem, `{"matches":["root_test.go"]}`),
						messageRecord("assistant-2", message.KindAssistant, "hello"),
					},
					Transition: "assistant_completed",
				},
			},
		}
		return nil
	}

	model := NewRootModel(svc)

	model, listenCmd := bootstrapRestoredSession(t, model)

	model.composer.value = "hello"

	updated, sendCmd := model.Update(enterKey())
	model = updated.(rootModel)

	if !model.loading {
		t.Fatal("expected loading state after enter")
	}

	_ = sendCmd()

	for i := 0; i < 4; i++ {
		updated, listenCmd = model.Update(listenCmd())
		model = updated.(rootModel)
		if !model.loading {
			t.Fatalf("expected loading to continue during multi-turn event %d", i)
		}
	}

	updated, listenCmd = model.Update(listenCmd())
	model = updated.(rootModel)

	if model.loading {
		t.Fatal("expected loading to stop after completion event")
	}
	if svc.eventsCalls != 1 {
		t.Fatalf("expected root model to subscribe once, got %d subscriptions", svc.eventsCalls)
	}
	if svc.lastSessionID != "session-1" {
		t.Fatalf("expected query to use restored session, got %q", svc.lastSessionID)
	}
	if svc.lastPrompt != "hello" {
		t.Fatalf("expected query prompt to be hello, got %q", svc.lastPrompt)
	}
	if len(model.messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(model.messages))
	}
	if textContent(model.messages[3].Parts) != "hello" {
		t.Fatalf("expected final assistant content, got %q", textContent(model.messages[3].Parts))
	}
}

func TestStreamFailureShowsErrorAndKeepsPartialAssistantMessage(t *testing.T) {
	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) error {
		svc.events <- app.BaseEvent{
			T: app.EventSession,
			Payload: session.SessionEvent{
				Status: session.StatusRestored,
				Session: &sessioncontract.Session{
					ID:    "session-1",
					Title: "demo",
				},
			},
		}
		return nil
	}
	svc.runQueryFn = func(_ context.Context, sessionID string, prompt string) error {
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

	model, listenCmd := bootstrapRestoredSession(t, model)

	model.composer.value = "hello"
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
	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) error {
		svc.events <- app.BaseEvent{
			T: app.EventSession,
			Payload: session.SessionEvent{
				Status: session.StatusRestored,
				Session: &sessioncontract.Session{
					ID:    "session-1",
					Title: "demo",
				},
			},
		}
		return nil
	}

	model := NewRootModel(svc)

	model, listenCmd := bootstrapRestoredSession(t, model)

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

	updated, _ := model.Update(listenCmd())
	model = updated.(rootModel)

	if model.loading {
		t.Fatal("expected loading to stop after query failure")
	}
	if model.errMessage != "query failed" {
		t.Fatalf("expected query failure error, got %q", model.errMessage)
	}
}

func TestQueryCompletedEventStopsLoading(t *testing.T) {
	svc := newStubChatService()
	svc.ensureSessionFn = func(_ context.Context) error {
		svc.events <- app.BaseEvent{
			T: app.EventSession,
			Payload: session.SessionEvent{
				Status: session.StatusRestored,
				Session: &sessioncontract.Session{
					ID:    "session-1",
					Title: "demo",
				},
			},
		}
		return nil
	}

	model := NewRootModel(svc)

	model, listenCmd := bootstrapRestoredSession(t, model)

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

	updated, _ := model.Update(listenCmd())
	model = updated.(rootModel)

	if model.loading {
		t.Fatal("expected loading to stop after query completion")
	}
}

type stubChatService struct {
	events               chan app.Event
	eventsCalls          int
	ensureSessionFn      func(ctx context.Context) error
	runQueryFn           func(ctx context.Context, sessionID string, prompt string) error
	history              []message.Message
	lastHistorySessionID string
	lastSessionID        string
	lastPrompt           string
}

type stubMarkdownRenderer struct {
	calls    int
	inputs   []string
	rendered string
	err      error
}

func (r *stubMarkdownRenderer) Render(input string) (string, error) {
	r.calls++
	r.inputs = append(r.inputs, input)
	if r.err != nil {
		return "", r.err
	}
	return r.rendered, nil
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

func (s *stubChatService) RunQuery(ctx context.Context, sessionID string, prompt string) error {
	s.lastSessionID = sessionID
	s.lastPrompt = prompt
	if s.runQueryFn == nil {
		return nil
	}
	return s.runQueryFn(ctx, sessionID, prompt)
}

func (s *stubChatService) ListHistory(_ context.Context, sessionID string) ([]message.Message, error) {
	s.lastHistorySessionID = sessionID
	return append([]message.Message(nil), s.history...), nil
}

func (s *stubChatService) Events() <-chan app.Event {
	s.eventsCalls++
	return s.events
}

func (s *stubChatService) InitializePermissionLevel(_ context.Context) error {
	if lifecycle.State == nil {
		lifecycle.State = &lifecycle.GlobalState{}
	}
	lifecycle.State.PermissionLevel = lifecycle.SafeLevel
	return nil
}

func enterKey() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
}

func bootstrapRestoredSession(t *testing.T, model rootModel) (rootModel, tea.Cmd) {
	t.Helper()

	bootstrapMsg := model.Init()()
	updated, listenCmd := model.Update(bootstrapMsg)
	model = updated.(rootModel)

	updated, historyCmd := model.Update(listenCmd())
	model = updated.(rootModel)

	updated, listenCmd = model.Update(historyCmd())
	return updated.(rootModel), listenCmd
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
