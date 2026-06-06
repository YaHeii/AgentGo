package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/agent"
	agentcontract "github.com/YaHeii/agentGo/internal/agent/contract"
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/lifecycle"
	message "github.com/YaHeii/agentGo/internal/message/contract"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/session"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
	uv "github.com/charmbracelet/ultraviolet"
)

func TestHeaderViewUsesLifecycleStateAndTransientStatus(t *testing.T) {
	t.Cleanup(func() {
		lifecycle.State = nil
	})

	lifecycle.State = &lifecycle.GlobalState{
		SessionID:               "session-1",
		Cwd:                     "/root/agentGo/internal/ui/model",
		ProjectRoot:             "/root/agentGo",
		Model:                   "gpt-test",
		PermissionLevel:         lifecycle.AttentionLevel,
		CurrentTurnInputTokens:  11,
		CurrentTurnOutputTokens: 7,
		CurrentTurnTotalTokens:  18,
		ActualContextTokens:     64,
		CumulativeInputTokens:   101,
		CumulativeOutputTokens:  52,
		CumulativeTotalTokens:   153,
		CurrentMessageCount:     6,
	}

	header := newHeader()
	header.SetTransientStatus("assistant is thinking...")

	view := header.View(80)
	lines := strings.Split(view, "\n")
	if len(lines) != header.Height() {
		t.Fatalf("expected %d header lines, got %d", header.Height(), len(lines))
	}
	if !strings.Contains(lines[0], "session-1") {
		t.Fatalf("expected session line, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "internal/ui/model") {
		t.Fatalf("expected relative cwd, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "gpt-test") || !strings.Contains(lines[2], "attention") {
		t.Fatalf("expected model/permission line, got %q", lines[2])
	}
	if !strings.Contains(lines[3], "11") || !strings.Contains(lines[3], "64") {
		t.Fatalf("expected current token/context line, got %q", lines[3])
	}
	if !strings.Contains(lines[4], "153") || !strings.Contains(lines[4], "6") {
		t.Fatalf("expected cumulative/message line, got %q", lines[4])
	}
	if lines[5] != "assistant is thinking..." {
		t.Fatalf("expected transient status line, got %q", lines[5])
	}
}

func TestHeaderViewLeavesLifecycleFieldsBlankAndKeepsDefaultStatus(t *testing.T) {
	t.Cleanup(func() {
		lifecycle.State = nil
	})
	lifecycle.State = nil

	header := newHeader()
	view := header.View(40)
	lines := strings.Split(view, "\n")
	if len(lines) != header.Height() {
		t.Fatalf("expected %d lines, got %d", header.Height(), len(lines))
	}
	if lines[0] != "agent: " {
		t.Fatalf("expected blank agent/session line, got %q", lines[0])
	}
	if lines[5] != defaultHeaderStatus {
		t.Fatalf("expected default transient status %q, got %q", defaultHeaderStatus, lines[5])
	}
}

func TestHeaderViewEllipsizesLongCWDFromLeft(t *testing.T) {
	t.Cleanup(func() {
		lifecycle.State = nil
	})

	lifecycle.State = &lifecycle.GlobalState{
		Cwd:         "/root/agentGo/very/long/path/to/internal/ui/model",
		ProjectRoot: "/root/agentGo",
	}

	header := newHeader()
	view := header.View(24)
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[1], "...") {
		t.Fatalf("expected left ellipsis in cwd line, got %q", lines[1])
	}
	if !strings.HasSuffix(lines[1], "internal/ui/model") {
		t.Fatalf("expected cwd suffix preserved, got %q", lines[1])
	}
}

func TestInputAppendBackspaceClearAndSlashFilter(t *testing.T) {
	input := newInput()
	input.Append("/")
	input.Append("per")

	if input.Value() != "/per" {
		t.Fatalf("expected input value /per, got %q", input.Value())
	}
	filter, ok := input.SlashFilter()
	if !ok || filter != "per" {
		t.Fatalf("expected slash filter per, got %q %v", filter, ok)
	}

	input.Backspace()
	if input.Value() != "/pe" {
		t.Fatalf("expected input value /pe after backspace, got %q", input.Value())
	}

	input.Clear()
	if input.Value() != "" {
		t.Fatalf("expected input cleared, got %q", input.Value())
	}
	if _, ok := input.SlashFilter(); ok {
		t.Fatal("expected slash filter closed after clear")
	}
}

func TestInputUpdateUsesTextareaEditing(t *testing.T) {
	input := newInput()

	updated, _ := input.Update(tea.KeyPressMsg(tea.Key{Text: "h", Code: 'h'}))
	input = updated
	updated, _ = input.Update(tea.KeyPressMsg(tea.Key{Text: "i", Code: 'i'}))
	input = updated
	updated, _ = input.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	input = updated

	if input.Value() != "h" {
		t.Fatalf("expected textarea-backed input value h, got %q", input.Value())
	}
}

func TestInputHeightAndViewWrapContent(t *testing.T) {
	input := newInput()
	input.Append("hello world from input")

	if input.Height(10) <= 1 {
		t.Fatalf("expected wrapped height > 1, got %d", input.Height(10))
	}

	view := input.View(10)
	if !strings.Contains(view, "\n") {
		t.Fatalf("expected wrapped input view, got %q", view)
	}
	if !strings.Contains(view, inputPrefix) {
		t.Fatalf("expected input prefix, got %q", view)
	}
}

func TestGenerateLayoutUsesHeaderInputAndSlashMenuHeights(t *testing.T) {
	ui := New(newStubAppService())
	ui.width = 80
	ui.height = 24
	ui.input.Append("hello world from input")
	ui.slashMenu.Open("per", 80)
	ui.recomputeLayout()

	layout := ui.layout
	if layout.header.Dy() != ui.header.Height() {
		t.Fatalf("expected header height %d, got %d", ui.header.Height(), layout.header.Dy())
	}
	if layout.input.Dy() != ui.input.Height(80) {
		t.Fatalf("expected input height %d, got %d", ui.input.Height(80), layout.input.Dy())
	}
	if layout.slashMenu.Dy() != slashMenuHeight {
		t.Fatalf("expected slash menu height %d, got %d", slashMenuHeight, layout.slashMenu.Dy())
	}
	expectedMainHeight := 24 - ui.header.Height() - ui.input.Height(80) - slashMenuHeight
	if layout.main.Dy() != expectedMainHeight {
		t.Fatalf("expected main height %d, got %d", expectedMainHeight, layout.main.Dy())
	}
}

func TestLocalMouseConvertsGlobalCoordinatesToRegionLocalCoordinates(t *testing.T) {
	ui := New(newStubAppService())
	ui.width = 80
	ui.height = 24
	ui.recomputeLayout()

	msg := tea.MouseClickMsg(tea.Mouse{
		X:      ui.layout.main.Min.X + 3,
		Y:      ui.layout.main.Min.Y + 2,
		Button: tea.MouseLeft,
	})

	region, local := ui.localMouse(msg)
	if region != uiRegionMain {
		t.Fatalf("expected main region, got %q", region)
	}
	mouse := local.Mouse()
	if mouse.X != 3 || mouse.Y != 2 {
		t.Fatalf("expected local coordinates 3,2 got %d,%d", mouse.X, mouse.Y)
	}
}

func TestUpdateEnterSendsMessageAndClearsInput(t *testing.T) {
	svc := newStubAppService()
	ui := New(svc)
	ui.sessionID = "session-1"
	ui.input.Append("hello")

	updated, cmd := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)

	if cmd == nil {
		t.Fatal("expected send command")
	}
	if ui.input.Value() != "" {
		t.Fatalf("expected input cleared, got %q", ui.input.Value())
	}
	if !ui.chat.loading {
		t.Fatal("expected loading state after send")
	}
}

func TestUpdateKeypadEnterSendsMessageAndClearsInput(t *testing.T) {
	svc := newStubAppService()
	ui := New(svc)
	ui.sessionID = "session-1"
	ui.input.Append("hello")

	updated, cmd := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyKpEnter}))
	ui = updated.(*UI)

	if cmd == nil {
		t.Fatal("expected send command")
	}
	if ui.input.Value() != "" {
		t.Fatalf("expected input cleared, got %q", ui.input.Value())
	}
	if !ui.chat.loading {
		t.Fatal("expected loading state after send")
	}
}

func TestUpdateBackspaceRoutesToInput(t *testing.T) {
	ui := New(newStubAppService())
	ui.input.Append("hi")

	updated, _ := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	ui = updated.(*UI)

	if ui.input.Value() != "h" {
		t.Fatalf("expected input to handle backspace, got %q", ui.input.Value())
	}
}

func TestUpdateSlashMenuEnterKeepsCompactExplicitlyUnimplemented(t *testing.T) {
	ui := New(newStubAppService())
	ui.width = 80
	ui.height = 24

	ui.input.Append("/compact")
	ui.syncSlashMenuFromInput()

	if !ui.slashMenu.IsOpen() {
		t.Fatal("expected slash menu open")
	}

	updated, _ := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)

	if ui.slashMenu.IsOpen() {
		t.Fatal("expected slash menu to close after execution")
	}
	if !strings.Contains(ui.header.TransientStatus(), "not implemented: /compact") {
		t.Fatalf("expected transient slash status, got %q", ui.header.TransientStatus())
	}
}

func TestUpdateSlashMenuKeypadEnterKeepsCompactExplicitlyUnimplemented(t *testing.T) {
	ui := New(newStubAppService())
	ui.width = 80
	ui.height = 24

	ui.input.Append("/compact")
	ui.syncSlashMenuFromInput()

	if !ui.slashMenu.IsOpen() {
		t.Fatal("expected slash menu open")
	}

	updated, _ := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyKpEnter}))
	ui = updated.(*UI)

	if ui.slashMenu.IsOpen() {
		t.Fatal("expected slash menu to close after execution")
	}
	if !strings.Contains(ui.header.TransientStatus(), "not implemented: /compact") {
		t.Fatalf("expected transient slash status, got %q", ui.header.TransientStatus())
	}
}

func TestHistorySessionCommandListsSessionsAndSwitchesSelection(t *testing.T) {
	svc := newStubAppService()
	svc.sessions = []sessioncontract.Session{
		{ID: "session-1", Title: "First"},
		{ID: "session-2", Title: "Second"},
	}
	ui := New(svc)
	ui.width = 80
	ui.height = 24

	updated, _ := ui.Update(tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	ui = updated.(*UI)
	updated, cmd := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)
	if cmd == nil {
		t.Fatal("expected sessions load command")
	}

	updated, _ = ui.Update(cmd())
	ui = updated.(*UI)
	if !ui.slashMenu.IsOpen() || !strings.Contains(ui.slashMenu.View(), "Second") {
		t.Fatalf("expected session list view, got %q", ui.slashMenu.View())
	}

	updated, _ = ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	ui = updated.(*UI)
	updated, cmd = ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)
	if cmd == nil {
		t.Fatal("expected switch session command")
	}

	updated, _ = ui.Update(cmd())
	ui = updated.(*UI)
	if svc.lastSwitchSessionID != "session-2" {
		t.Fatalf("expected switched session-2, got %q", svc.lastSwitchSessionID)
	}
	if ui.slashMenu.IsOpen() {
		t.Fatal("expected slash menu closed after switch")
	}
}

func TestPermissionCommandUpdatesLifecyclePermissionLevel(t *testing.T) {
	t.Cleanup(func() {
		lifecycle.State = nil
	})
	lifecycle.State = &lifecycle.GlobalState{PermissionLevel: lifecycle.SafeLevel}

	ui := New(newStubAppService())
	ui.width = 80
	ui.height = 24
	ui.input.Append("/per")
	ui.syncSlashMenuFromInput()
	updated, _ := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)

	if !ui.slashMenu.IsOpen() || !strings.Contains(ui.slashMenu.View(), "attention") {
		t.Fatalf("expected permission list view, got %q", ui.slashMenu.View())
	}

	updated, _ = ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	ui = updated.(*UI)
	updated, _ = ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)

	if lifecycle.State.PermissionLevel != lifecycle.AttentionLevel {
		t.Fatalf("expected attention permission, got %v", lifecycle.State.PermissionLevel)
	}
	if ui.slashMenu.IsOpen() {
		t.Fatal("expected slash menu closed")
	}
}

func TestNewSessionCommandStartsNewSession(t *testing.T) {
	svc := newStubAppService()
	ui := New(svc)
	ui.width = 80
	ui.height = 24
	ui.input.Append("/new")
	ui.syncSlashMenuFromInput()

	updated, cmd := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)
	if cmd == nil {
		t.Fatal("expected new session command")
	}

	updated, _ = ui.Update(cmd())
	ui = updated.(*UI)
	if svc.lastNewSessionTitle != "New Session" {
		t.Fatalf("expected New Session title, got %q", svc.lastNewSessionTitle)
	}
	if ui.slashMenu.IsOpen() {
		t.Fatal("expected slash menu closed")
	}
}

func TestCompactCommandRemainsExplicitlyUnimplemented(t *testing.T) {
	ui := New(newStubAppService())
	ui.width = 80
	ui.height = 24
	ui.input.Append("/compact")
	ui.syncSlashMenuFromInput()

	updated, cmd := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)
	if cmd != nil {
		t.Fatal("expected no command for compact")
	}
	if !strings.Contains(ui.header.TransientStatus(), "not implemented: /compact") {
		t.Fatalf("expected compact not implemented status, got %q", ui.header.TransientStatus())
	}
}

func TestInitUsesLifecycleSessionWithoutLoadingHistory(t *testing.T) {
	t.Cleanup(func() {
		lifecycle.State = nil
	})
	lifecycle.State = &lifecycle.GlobalState{SessionID: "session-1"}

	svc := newStubAppService()
	svc.history = []message.Message{
		messageRecord("u1", message.KindUser, "hello"),
		messageRecord("a1", message.KindAssistant, "world"),
	}

	ui := New(svc)

	initBatch, ok := ui.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected init batch message, got %T", ui.Init()())
	}
	if len(initBatch) != 2 {
		t.Fatalf("expected 2 init commands, got %d", len(initBatch))
	}

	if ui.sessionID != "session-1" {
		t.Fatalf("expected active session, got %q", ui.sessionID)
	}
	if svc.lastHistorySessionID != "" {
		t.Fatalf("expected init not to load history, got %q", svc.lastHistorySessionID)
	}
	if len(ui.chat.messages) != 0 {
		t.Fatalf("expected empty transcript on startup, got %d messages", len(ui.chat.messages))
	}
}

func TestInitReadsLifecycleSessionAndEnterCanSendImmediately(t *testing.T) {
	t.Cleanup(func() {
		lifecycle.State = nil
	})
	lifecycle.State = &lifecycle.GlobalState{SessionID: "session-1"}

	svc := newStubAppService()
	ui := New(svc)
	_ = ui.Init()
	ui.input.Append("hello")

	updated, cmd := ui.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	ui = updated.(*UI)

	if cmd == nil {
		t.Fatal("expected send command")
	}
	if ui.input.Value() != "" {
		t.Fatalf("expected input cleared, got %q", ui.input.Value())
	}
	if ui.header.TransientStatus() != "assistant is thinking..." {
		t.Fatalf("expected transient status updated, got %q", ui.header.TransientStatus())
	}
}

func TestInitAndSubsequentSessionSwitchLoadsHistory(t *testing.T) {
	t.Cleanup(func() {
		lifecycle.State = nil
	})
	lifecycle.State = &lifecycle.GlobalState{SessionID: "session-start"}

	svc := newStubAppService()
	svc.history = []message.Message{
		messageRecord("u1", message.KindUser, "hello"),
		messageRecord("a1", message.KindAssistant, "world"),
	}
	ui := New(svc)

	initBatch, ok := ui.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected init batch message, got %T", ui.Init()())
	}

	svc.events <- app.BaseEvent{
		T: app.EventSession,
		Payload: session.SessionEvent{
			Status: session.StatusSwitched,
			Session: &sessioncontract.Session{
				ID:    "session-2",
				Title: "demo",
			},
		},
	}

	updated, historyCmd := ui.Update(initBatch[1]())
	ui = updated.(*UI)
	updated, _ = ui.Update(historyCmd())
	ui = updated.(*UI)

	if ui.sessionID != "session-2" {
		t.Fatalf("expected switched session id, got %q", ui.sessionID)
	}
	if svc.lastHistorySessionID != "session-2" {
		t.Fatalf("expected switched history load, got %q", svc.lastHistorySessionID)
	}
	if len(ui.chat.messages) != 2 {
		t.Fatalf("expected 2 hydrated messages, got %d", len(ui.chat.messages))
	}
}

func TestUIReusesSingleEventSubscription(t *testing.T) {
	t.Cleanup(func() {
		lifecycle.State = nil
	})
	lifecycle.State = &lifecycle.GlobalState{SessionID: "session-1"}

	svc := newStubAppService()
	ui := New(svc)

	initBatch, ok := ui.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected init batch message, got %T", ui.Init()())
	}

	svc.events <- app.BaseEvent{
		T: app.EventSession,
		Payload: session.SessionEvent{
			Status:  session.StatusSwitched,
			Session: &sessioncontract.Session{ID: "session-2"},
		},
	}

	updated, historyCmd := ui.Update(initBatch[1]())
	ui = updated.(*UI)
	updated, listenCmd := ui.Update(historyCmd())
	ui = updated.(*UI)

	svc.events <- app.BaseEvent{
		T: app.EventSession,
		Payload: session.SessionEvent{
			Status:  session.StatusRestored,
			Session: &sessioncontract.Session{ID: "session-3"},
		},
	}

	updated, _ = ui.Update(listenCmd())
	ui = updated.(*UI)

	if svc.eventsCalls != 1 {
		t.Fatalf("expected a single event subscription, got %d", svc.eventsCalls)
	}
}

func TestAgentCompletedEventRendersMarkdownAndStopsLoading(t *testing.T) {
	renderer := &stubMarkdownRenderer{rendered: "rendered markdown"}
	ui := New(newStubAppService())
	ui.chat.renderer = renderer
	ui.chat.quietRenderer = &stubMarkdownRenderer{rendered: "quiet markdown"}
	ui.chat.loading = true

	event := appEventMsg{
		event: app.BaseEvent{
			T: app.EventAgent,
			Payload: agent.QueryEvent{
				Status: agentcontract.LoopCompleted,
				State: agent.LoopState{
					Messages: []message.Message{
						messageRecord("assistant-1", message.KindAssistant, "**final**"),
					},
				},
			},
		},
	}

	updated, _ := ui.Update(event)
	ui = updated.(*UI)

	if ui.chat.loading {
		t.Fatal("expected loading to stop")
	}
	if ui.header.TransientStatus() != defaultHeaderStatus {
		t.Fatalf("expected default header status, got %q", ui.header.TransientStatus())
	}
	if renderer.calls != 1 {
		t.Fatalf("expected markdown render, got %d", renderer.calls)
	}
}

func TestProviderTextDeltaUpdatesActiveAssistantMessage(t *testing.T) {
	ui := New(newStubAppService())
	ui.chat.loading = true

	started := appEventMsg{
		event: app.BaseEvent{
			T: app.EventAgent,
			Payload: agent.QueryEvent{
				Status: agentcontract.LoopStarted,
				State: agent.LoopState{
					Messages: []message.Message{
						messageRecord("user-1", message.KindUser, "hello"),
						messageRecord("assistant-1", message.KindAssistant, ""),
					},
				},
			},
		},
	}
	updated, _ := ui.Update(started)
	ui = updated.(*UI)

	for _, delta := range []string{"hel", "lo"} {
		updated, _ = ui.Update(appEventMsg{
			event: app.BaseEvent{
				T: app.EventProvider,
				Payload: provider.StreamEvent{
					Type:      provider.StreamEventTextDelta,
					TextDelta: delta,
				},
			},
		})
		ui = updated.(*UI)
	}

	if got := textContent(ui.chat.messages[1].Parts); got != "hello" {
		t.Fatalf("expected streamed assistant text hello, got %q", got)
	}
	if !strings.Contains(ui.chat.viewport.View(), "hello") {
		t.Fatalf("expected transcript to show streamed text, got %q", ui.chat.viewport.View())
	}
	if !ui.chat.loading {
		t.Fatal("expected provider delta to keep loading state")
	}
}

func TestSendMessageDoneErrorSetsTransientStatus(t *testing.T) {
	ui := New(newStubAppService())
	ui.chat.loading = true

	updated, _ := ui.Update(sendMessageDoneMsg{err: errors.New("stream failed")})
	ui = updated.(*UI)

	if ui.chat.loading {
		t.Fatal("expected loading to stop")
	}
	if ui.header.TransientStatus() != "stream failed" {
		t.Fatalf("expected error transient status, got %q", ui.header.TransientStatus())
	}
}

type stubAppService struct {
	events               chan app.Event
	eventsCalls          int
	runQueryFn           func(ctx context.Context, sessionID string, prompt string) error
	history              []message.Message
	sessions             []sessioncontract.Session
	lastHistorySessionID string
	lastSwitchSessionID  string
	lastNewSessionTitle  string
	lastSessionID        string
	lastPrompt           string
	listSessionsErr      error
	switchSessionErr     error
	startNewSessionErr   error
}

func newStubAppService() *stubAppService {
	return &stubAppService{events: make(chan app.Event, 16)}
}

func (s *stubAppService) ListHistory(_ context.Context, sessionID string) ([]message.Message, error) {
	s.lastHistorySessionID = sessionID
	return append([]message.Message(nil), s.history...), nil
}

func (s *stubAppService) RunQuery(ctx context.Context, sessionID string, prompt string) error {
	s.lastSessionID = sessionID
	s.lastPrompt = prompt
	if s.runQueryFn == nil {
		return nil
	}
	return s.runQueryFn(ctx, sessionID, prompt)
}

func (s *stubAppService) ListSessions(context.Context) ([]sessioncontract.Session, error) {
	return append([]sessioncontract.Session(nil), s.sessions...), s.listSessionsErr
}

func (s *stubAppService) SwitchSession(_ context.Context, sessionID string) error {
	s.lastSwitchSessionID = sessionID
	return s.switchSessionErr
}

func (s *stubAppService) StartNewSession(_ context.Context, title string) error {
	s.lastNewSessionTitle = title
	return s.startNewSessionErr
}

func (s *stubAppService) Events() <-chan app.Event {
	s.eventsCalls++
	return s.events
}

type stubMarkdownRenderer struct {
	rendered string
	calls    int
}

func (s *stubMarkdownRenderer) Render(_ string) (string, error) {
	s.calls++
	return s.rendered, nil
}

func messageRecord(id string, kind message.Kind, text string) message.Message {
	return message.Message{
		ID:        id,
		SessionID: "session-1",
		Kind:      kind,
		Parts: []message.Part{
			{Type: message.PartTypeText, Text: text},
		},
	}
}

func TestWrapPrefixedTextPreservesPrefixOnWrappedLines(t *testing.T) {
	lines := wrapPrefixedText("hello world", "> ", "  ", 6)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped lines, got %#v", lines)
	}
	if !strings.HasPrefix(lines[0], "> ") {
		t.Fatalf("expected first prefix, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  ") {
		t.Fatalf("expected continuation prefix, got %q", lines[1])
	}
}

func TestNewInitializesChatState(t *testing.T) {
	ui := New(newStubAppService())
	if ui.state != uiStateChat {
		t.Fatalf("expected chat state, got %q", ui.state)
	}
	if ui.header.Height() != headerHeight {
		t.Fatalf("expected header height %d, got %d", headerHeight, ui.header.Height())
	}
	if ui.input.Value() != "" {
		t.Fatalf("expected empty input, got %q", ui.input.Value())
	}
	if ui.layout.header.Dx() != 0 || ui.layout.main.Dx() != 0 {
		t.Fatalf("expected zero layout before sizing, got %#v", ui.layout)
	}
}

func TestViewUsesMouseModeAndDrawPath(t *testing.T) {
	ui := New(newStubAppService())
	ui.width = 40
	ui.height = 16
	ui.recomputeLayout()

	view := ui.View()
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("expected mouse mode cell motion, got %v", view.MouseMode)
	}
	if view.Content == "" {
		t.Fatal("expected rendered content")
	}
}

func TestDrawRendersInputAndHeader(t *testing.T) {
	t.Cleanup(func() {
		lifecycle.State = nil
	})
	lifecycle.State = &lifecycle.GlobalState{Model: "gpt-test"}

	ui := New(newStubAppService())
	ui.width = 40
	ui.height = 16
	ui.input.Append("hi")
	ui.recomputeLayout()

	screen := uv.NewScreenBuffer(40, 16)
	ui.Draw(screen, screen.Bounds())
	rendered := screen.Render()

	if !strings.Contains(rendered, "gpt-test") {
		t.Fatalf("expected header content, got %q", rendered)
	}
	if !strings.Contains(rendered, inputPrefix) || !strings.Contains(rendered, "hi") {
		t.Fatalf("expected input content, got %q", rendered)
	}
}
