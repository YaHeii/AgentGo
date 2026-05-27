package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/app"
	message "github.com/YaHeii/agentGo/internal/message/contract"
	"github.com/YaHeii/agentGo/internal/session"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
	uv "github.com/charmbracelet/ultraviolet"
)

func TestNewInitializesDefaultStateAndFocus(t *testing.T) {
	ui := New(newStubAppService())

	if ui.state != uiStateChat {
		t.Fatalf("expected default ui state chat, got %q", ui.state)
	}
	if ui.focus != uiFocusEditor {
		t.Fatalf("expected default focus editor, got %q", ui.focus)
	}
	if ui.layout.header.Dx() != 0 || ui.layout.main.Dx() != 0 {
		t.Fatalf("expected zero layout before sizing, got %#v", ui.layout)
	}
}

func TestGenerateLayoutCreatesHeaderMainStatusEditorAndSlashMenuAreas(t *testing.T) {
	ui := New(newStubAppService())
	layout := ui.generateLayout(uv.Rect(0, 0, 80, 24))

	if layout.header.Dy() <= 0 {
		t.Fatalf("expected header height > 0, got %v", layout.header)
	}
	if layout.main.Dy() <= 0 {
		t.Fatalf("expected main height > 0, got %v", layout.main)
	}
	if layout.status.Dy() <= 0 {
		t.Fatalf("expected status height > 0, got %v", layout.status)
	}
	if layout.editor.Dy() <= 0 {
		t.Fatalf("expected editor height > 0, got %v", layout.editor)
	}
	if layout.slashMenu.Min.X < 0 || layout.slashMenu.Min.Y < 0 {
		t.Fatalf("expected slash menu rectangle to be initialized, got %v", layout.slashMenu)
	}
	if layout.main.Min.Y < layout.header.Max.Y {
		t.Fatalf("expected main below header, got header=%v main=%v", layout.header, layout.main)
	}
}

func TestDrawRendersHeaderAndComposerIntoScreenBuffer(t *testing.T) {
	ui := New(newStubAppService())
	ui.width = 40
	ui.height = 12
	ui.layout = ui.generateLayout(uv.Rect(0, 0, 40, 12))
	ui.chat.composer = ui.chat.composer.Update(tea.KeyPressMsg(tea.Key{Text: "h"}))
	ui.chat.composer = ui.chat.composer.Update(tea.KeyPressMsg(tea.Key{Text: "i"}))

	screen := uv.NewScreenBuffer(40, 12)
	ui.Draw(screen, screen.Bounds())

	rendered := screen.Render()
	if !strings.Contains(rendered, "agentGo") {
		t.Fatalf("expected header drawn, got %q", rendered)
	}
	if !strings.Contains(rendered, "> hi") {
		t.Fatalf("expected composer drawn, got %q", rendered)
	}
}

func TestViewUsesDrawCompatibilityPath(t *testing.T) {
	ui := New(newStubAppService())
	ui.width = 40
	ui.height = 12
	ui.layout = ui.generateLayout(uv.Rect(0, 0, 40, 12))

	view := ui.View()
	if !strings.Contains(view.Content, "agentGo") {
		t.Fatalf("expected header in compatibility view, got %q", view.Content)
	}
}

func TestInitBootstrapsSessionAndLoadsHistory(t *testing.T) {
	svc := newStubAppService()
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

	ui := New(svc)

	initMsg := ui.Init()()
	updated, listenCmd := ui.Update(initMsg)
	ui = updated.(*UI)

	updated, historyCmd := ui.Update(listenCmd())
	ui = updated.(*UI)
	updated, _ = ui.Update(historyCmd())
	ui = updated.(*UI)

	if ui.chat.sessionID != "session-1" {
		t.Fatalf("expected active session, got %q", ui.chat.sessionID)
	}
	if len(ui.chat.messages) != 2 {
		t.Fatalf("expected 2 hydrated messages, got %d", len(ui.chat.messages))
	}
}

func TestUIComposesChatComponents(t *testing.T) {
	ui := New(newStubAppService())

	if ui.chat.transcript.viewport.Width != defaultWidth {
		t.Fatalf("expected transcript viewport width %d, got %d", defaultWidth, ui.chat.transcript.viewport.Width)
	}
	if ui.chat.composer.value != "" {
		t.Fatalf("expected empty composer, got %q", ui.chat.composer.value)
	}
	if ui.chat.status.message != "" {
		t.Fatalf("expected empty status message, got %q", ui.chat.status.message)
	}

	updated, _ := ui.Update(tea.KeyPressMsg(tea.Key{Text: "h"}))
	ui = updated.(*UI)

	if ui.chat.composer.value != "h" {
		t.Fatalf("expected composer to hold input, got %q", ui.chat.composer.value)
	}
}

func TestUILoadsHistoryAfterSessionRestore(t *testing.T) {
	svc := newStubAppService()
	svc.history = []message.Message{
		messageRecord("history-user", message.KindUser, "old question"),
		messageRecord("history-assistant", message.KindAssistant, "old answer"),
	}

	ui := New(svc)
	updated, listenCmd := ui.Update(bootstrapDoneMsg{})
	ui = updated.(*UI)

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

	updated, historyCmd := ui.Update(listenCmd())
	ui = updated.(*UI)
	if ui.chat.sessionID != "session-1" {
		t.Fatalf("expected restored session, got %q", ui.chat.sessionID)
	}

	updated, _ = ui.Update(historyCmd())
	ui = updated.(*UI)

	if svc.lastHistorySessionID != "session-1" {
		t.Fatalf("expected history load for session-1, got %q", svc.lastHistorySessionID)
	}
	if len(ui.chat.messages) != 2 {
		t.Fatalf("expected 2 history messages, got %d", len(ui.chat.messages))
	}
	if len(ui.chat.transcript.items) != 2 {
		t.Fatalf("expected 2 transcript items, got %d", len(ui.chat.transcript.items))
	}
}

func TestTranscriptBuildsCollapsibleItemsForStructuredParts(t *testing.T) {
	messages := []message.Message{
		{
			ID:        "user-1",
			SessionID: "session-1",
			Kind:      message.KindUser,
			Parts: []message.Part{
				{Type: message.PartTypeText, Text: "   show me status   "},
			},
		},
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

	items := buildTranscriptItems(messages, nil, nil, nil)

	if len(items) != 4 {
		t.Fatalf("expected 4 transcript items, got %d", len(items))
	}
	if items[0].Summary != "you: show me status" {
		t.Fatalf("expected user summary trimmed to input, got %q", items[0].Summary)
	}
	if items[0].Body != "show me status" {
		t.Fatalf("expected user body trimmed to input, got %q", items[0].Body)
	}
	if items[0].Expanded {
		t.Fatal("expected user item to never be expandable")
	}
	if items[1].Summary != "ai: final answer" {
		t.Fatalf("expected assistant text summary, got %q", items[1].Summary)
	}
	if items[1].PartType != message.PartTypeText {
		t.Fatalf("expected assistant main item to be final text, got %q", items[1].PartType)
	}
	if !strings.Contains(items[1].Body, "expanded reasoning") {
		t.Fatalf("expected assistant detail body to include reasoning, got %q", items[1].Body)
	}
	if !strings.Contains(items[1].Body, "tool call: grep") {
		t.Fatalf("expected assistant detail body to include tool call, got %q", items[1].Body)
	}
	if !strings.Contains(items[1].Body, "tool result: call-1") {
		t.Fatalf("expected assistant detail body to include tool result, got %q", items[1].Body)
	}
	if items[2].Summary != "system: provider_error" {
		t.Fatalf("expected system summary, got %q", items[2].Summary)
	}
	if items[3].Summary != "progress: tool_call 1/3" {
		t.Fatalf("expected progress summary, got %q", items[3].Summary)
	}
}

func TestTranscriptBuildsAssistantProcessOnlyMessageAsCollapsibleSummary(t *testing.T) {
	messages := []message.Message{
		{
			ID:        "assistant-1",
			SessionID: "session-1",
			Kind:      message.KindAssistant,
			Parts: []message.Part{
				{
					Type: message.PartTypeThinking,
					Thinking: &message.ThinkingPart{
						Content: "reasoning only",
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
			},
		},
	}

	items := buildTranscriptItems(messages, nil, nil, nil)

	if len(items) != 1 {
		t.Fatalf("expected 1 transcript item, got %d", len(items))
	}
	if items[0].Summary != "ai: thinking / tool call: grep" {
		t.Fatalf("expected assistant process summary, got %q", items[0].Summary)
	}
	if !strings.Contains(items[0].Body, "reasoning only") {
		t.Fatalf("expected assistant process body to include reasoning, got %q", items[0].Body)
	}
}

func TestTranscriptNormalizesEscapedNewlinesInAssistantDetails(t *testing.T) {
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
						Content: "line1\\nline2",
					},
				},
				{
					Type: message.PartTypeToolCall,
					ToolCall: &message.ToolCallPart{
						ID:    "call-1",
						Name:  "grep",
						Input: "{\"pattern\":\"a\\\\nb\"}",
					},
				},
			},
		},
	}

	items := buildTranscriptItems(messages, nil, nil, nil)

	if len(items) != 1 {
		t.Fatalf("expected 1 transcript item, got %d", len(items))
	}
	if strings.Contains(items[0].Body, "\\n") {
		t.Fatalf("expected escaped newlines to be normalized, got %q", items[0].Body)
	}
	if !strings.Contains(items[0].Body, "line1\nline2") {
		t.Fatalf("expected actual newline in thinking body, got %q", items[0].Body)
	}
}

func TestTranscriptTogglesSelectedItemExpansion(t *testing.T) {
	items := []transcriptItem{
		{ID: "user-1:0:text", PartType: message.PartTypeText, Kind: message.KindUser, Summary: "you: hello", Body: "hello"},
		{ID: "assistant-1", PartType: message.PartTypeText, Kind: message.KindAssistant, Summary: "ai: hello", Body: "hello\nthinking\nexpanded reasoning"},
	}

	model := newTranscriptModel(defaultWidth, defaultHeight).setItems(items)
	model = model.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	if model.selected != 1 {
		t.Fatalf("expected selected item 1, got %d", model.selected)
	}

	model = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !model.expanded["assistant-1"] {
		t.Fatal("expected selected item to be expanded")
	}
	if !strings.Contains(model.View(), "expanded reasoning") {
		t.Fatalf("expected expanded body in transcript view, got %q", model.View())
	}

	model = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	if model.expanded["assistant-1"] {
		t.Fatal("expected selected item to collapse after space")
	}
}

func TestEnterPrefersTranscriptExpansionOverSubmit(t *testing.T) {
	ui := New(newStubAppService())
	ui.chat.sessionID = "session-1"
	ui.chat.composer.value = "pending send"
	ui.chat.transcript = ui.chat.transcript.setItems([]transcriptItem{
		{ID: "user-1", PartType: message.PartTypeText, Kind: message.KindUser, Summary: "you: hi", Body: "hi"},
		{ID: "assistant-1", PartType: message.PartTypeText, Kind: message.KindAssistant, Summary: "ai: hello", Body: "hello\nthinking\nexpanded reasoning"},
	})
	ui.chat.transcript.selected = 1

	updated, cmd := ui.Update(enterKey())
	ui = updated.(*UI)

	if cmd != nil {
		t.Fatal("expected no submit command when expanding transcript item")
	}
	if !ui.chat.transcript.expanded["assistant-1"] {
		t.Fatal("expected enter to expand selected assistant transcript item")
	}
	if ui.chat.composer.value != "pending send" {
		t.Fatalf("expected composer value unchanged, got %q", ui.chat.composer.value)
	}
}

func TestAllowsScrollingTranscriptWhenMessagesOverflowMainView(t *testing.T) {
	ui := New(newStubAppService())
	ui.width = 40
	ui.height = 8
	ui.focus = uiFocusMain
	ui.chat.transcript = ui.chat.transcript.updateSize(40, 3)
	ui.chat.transcript = ui.chat.transcript.setItems([]transcriptItem{
		{ID: "user-1", PartType: message.PartTypeText, Kind: message.KindUser, Summary: "you: one", Body: "one"},
		{ID: "assistant-1", PartType: message.PartTypeText, Kind: message.KindAssistant, Summary: "ai: two", Body: "two\nthinking\nreasoning one"},
		{ID: "assistant-2", PartType: message.PartTypeText, Kind: message.KindAssistant, Summary: "ai: three", Body: "three\nthinking\nreasoning two"},
		{ID: "assistant-3", PartType: message.PartTypeText, Kind: message.KindAssistant, Summary: "ai: four", Body: "four\nthinking\nreasoning three"},
		{ID: "assistant-4", PartType: message.PartTypeText, Kind: message.KindAssistant, Summary: "ai: five", Body: "five\nthinking\nreasoning four"},
	})

	initial := ui.View().Content
	if strings.Contains(initial, "ai: five") {
		t.Fatalf("expected overflow item hidden before scrolling, got %q", initial)
	}

	updated, _ := ui.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	ui = updated.(*UI)
	updated, _ = ui.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	ui = updated.(*UI)
	updated, _ = ui.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	ui = updated.(*UI)
	updated, _ = ui.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	ui = updated.(*UI)

	scrolled := ui.View().Content
	if !strings.Contains(scrolled, "ai: five") {
		t.Fatalf("expected overflow item visible after scrolling, got %q", scrolled)
	}
}

func TestMarkdownRendersOnlyOnAgentTerminalEvent(t *testing.T) {
	renderer := &stubMarkdownRenderer{rendered: "rendered markdown"}
	quietRenderer := &stubMarkdownRenderer{rendered: "quiet markdown"}
	ui := New(newStubAppService())
	ui.chat.renderer = renderer
	ui.chat.quietRenderer = quietRenderer

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

	updated, _ := ui.Update(deltaEvent)
	ui = updated.(*UI)

	if renderer.calls != 0 {
		t.Fatalf("expected no markdown render during delta, got %d", renderer.calls)
	}
	if ui.chat.markdown["assistant-1"] != "" {
		t.Fatalf("expected no markdown cache during delta, got %q", ui.chat.markdown["assistant-1"])
	}
	if ui.chat.transcript.items[0].Body != "**draft**" {
		t.Fatalf("expected raw body during delta, got %q", ui.chat.transcript.items[0].Body)
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

	updated, _ = ui.Update(completedEvent)
	ui = updated.(*UI)

	if renderer.calls != 1 {
		t.Fatalf("expected one markdown render on completion, got %d", renderer.calls)
	}
	if quietRenderer.calls != 0 {
		t.Fatalf("expected no quiet render for plain assistant text, got %d", quietRenderer.calls)
	}
	if ui.chat.markdown["assistant-1"] != "rendered markdown" {
		t.Fatalf("expected cached markdown, got %q", ui.chat.markdown["assistant-1"])
	}
	if ui.chat.transcript.items[0].Body != "rendered markdown" {
		t.Fatalf("expected rendered body after completion, got %q", ui.chat.transcript.items[0].Body)
	}
}

func TestHistoryLoadRendersMarkdownForStableUserMessages(t *testing.T) {
	renderer := &stubMarkdownRenderer{rendered: "rendered user markdown"}
	ui := New(newStubAppService())
	ui.chat.renderer = renderer
	ui.chat.quietRenderer = &stubMarkdownRenderer{rendered: "quiet markdown"}
	ui.chat.sessionID = "session-1"

	updated, _ := ui.Update(historyLoadedMsg{
		sessionID: "session-1",
		messages: []message.Message{
			messageRecord("user-1", message.KindUser, "**hello**"),
		},
	})
	ui = updated.(*UI)

	if renderer.calls != 1 {
		t.Fatalf("expected one markdown render for stable user message, got %d", renderer.calls)
	}
	if ui.chat.transcript.items[0].Body != "rendered user markdown" {
		t.Fatalf("expected rendered user body, got %q", ui.chat.transcript.items[0].Body)
	}
}

func TestTranscriptUsesQuietMarkdownForAssistantThinkingAndToolDetails(t *testing.T) {
	renderer := &stubMarkdownRenderer{rendered: "final markdown"}
	quietRenderer := &stubMarkdownRenderer{rendered: "detail markdown"}
	ui := New(newStubAppService())
	ui.chat.renderer = renderer
	ui.chat.quietRenderer = quietRenderer

	completedEvent := appEventMsg{
		event: app.BaseEvent{
			T: app.EventAgent,
			Payload: agent.QueryEvent{
				Status: agent.QueryStatusCompleted,
				State: agent.LoopState{
					Messages: []message.Message{
						{
							ID:        "assistant-1",
							SessionID: "session-1",
							Kind:      message.KindAssistant,
							Parts: []message.Part{
								{Type: message.PartTypeText, Text: "```go\nfmt.Println(\"hi\")\n```"},
								{
									Type: message.PartTypeThinking,
									Thinking: &message.ThinkingPart{
										Content: "- step 1\n- step 2",
									},
								},
								{
									Type: message.PartTypeToolResult,
									ToolResult: &message.ToolResultPart{
										ToolCallID: "call-1",
										Content:    "```json\n{\"ok\":true}\n```",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	updated, _ := ui.Update(completedEvent)
	ui = updated.(*UI)

	if renderer.calls != 1 {
		t.Fatalf("expected one primary markdown render, got %d", renderer.calls)
	}
	if quietRenderer.calls != 2 {
		t.Fatalf("expected quiet renderer for thinking and tool detail, got %d", quietRenderer.calls)
	}
	if !strings.Contains(ui.chat.transcript.items[0].Body, "final markdown") {
		t.Fatalf("expected rendered final body, got %q", ui.chat.transcript.items[0].Body)
	}
	if !strings.Contains(ui.chat.transcript.items[0].Body, "detail markdown") {
		t.Fatalf("expected rendered detail body, got %q", ui.chat.transcript.items[0].Body)
	}
}

func TestStateMachineKeepsNetworkIOInCommands(t *testing.T) {
	svc := newStubAppService()
	ui := New(svc)
	ui.chat.sessionID = "session-1"
	ui.chat.composer.value = "hello"

	updated, cmd := ui.Update(enterKey())
	ui = updated.(*UI)

	if svc.lastPrompt != "" {
		t.Fatalf("expected no RunQuery call during Update, got %q", svc.lastPrompt)
	}
	if cmd == nil {
		t.Fatal("expected runQueryCmd")
	}
	if !ui.chat.loading {
		t.Fatal("expected loading to start immediately after enter")
	}

	_ = cmd()

	if svc.lastPrompt != "hello" {
		t.Fatalf("expected RunQuery in command execution, got %q", svc.lastPrompt)
	}
}

func TestViewIsPure(t *testing.T) {
	renderer := &stubMarkdownRenderer{rendered: "rendered markdown"}
	quietRenderer := &stubMarkdownRenderer{rendered: "quiet markdown"}
	ui := New(newStubAppService())
	ui.chat.renderer = renderer
	ui.chat.quietRenderer = quietRenderer
	ui.width = 40
	ui.height = 12
	ui.chat.markdown["assistant-1"] = "cached markdown"
	ui.chat.transcript = ui.chat.transcript.updateSize(40, 3)
	ui.chat.transcript = ui.chat.transcript.setItems([]transcriptItem{
		{ID: "assistant-1:0:text", Summary: "ai: hello", Body: "cached markdown"},
		{ID: "assistant-1:1:thinking", Summary: "thinking", Body: "reasoning"},
	})
	ui.chat.transcript.selected = 1
	ui.chat.transcript.expanded["assistant-1:1:thinking"] = true
	ui.chat.status = ui.chat.status.setMessage("assistant is thinking...")

	beforeCalls := renderer.calls
	beforeSelected := ui.chat.transcript.selected
	beforeExpanded := ui.chat.transcript.expanded["assistant-1:1:thinking"]
	beforeMarkdown := ui.chat.markdown["assistant-1"]
	beforeLayout := ui.layout
	beforeViewportWidth := ui.chat.transcript.viewport.Width
	beforeViewportHeight := ui.chat.transcript.viewport.Height

	view := ui.View()

	if view.Content == "" {
		t.Fatal("expected non-empty view")
	}
	if renderer.calls != beforeCalls {
		t.Fatalf("expected View to not render markdown, got %d calls", renderer.calls-beforeCalls)
	}
	if quietRenderer.calls != 0 {
		t.Fatalf("expected View to not render quiet markdown, got %d calls", quietRenderer.calls)
	}
	if ui.chat.transcript.selected != beforeSelected {
		t.Fatalf("expected selected item unchanged, got %d", ui.chat.transcript.selected)
	}
	if ui.chat.transcript.expanded["assistant-1:1:thinking"] != beforeExpanded {
		t.Fatal("expected expanded state unchanged")
	}
	if ui.chat.markdown["assistant-1"] != beforeMarkdown {
		t.Fatalf("expected markdown cache unchanged, got %q", ui.chat.markdown["assistant-1"])
	}
	if ui.layout != beforeLayout {
		t.Fatalf("expected layout unchanged, got before=%v after=%v", beforeLayout, ui.layout)
	}
	if ui.chat.transcript.viewport.Width != beforeViewportWidth || ui.chat.transcript.viewport.Height != beforeViewportHeight {
		t.Fatalf("expected transcript viewport unchanged, got before=%dx%d after=%dx%d",
			beforeViewportWidth,
			beforeViewportHeight,
			ui.chat.transcript.viewport.Width,
			ui.chat.transcript.viewport.Height,
		)
	}
}

func TestEnterRunsQueryAndAppliesMultiTurnEvents(t *testing.T) {
	svc := newStubAppService()
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

	ui := New(svc)
	ui, listenCmd := bootstrapRestoredSession(t, ui)

	ui.chat.composer.value = "hello"
	updated, sendCmd := ui.Update(enterKey())
	ui = updated.(*UI)

	if !ui.chat.loading {
		t.Fatal("expected loading state after enter")
	}

	_ = sendCmd()

	for i := 0; i < 4; i++ {
		updated, listenCmd = ui.Update(listenCmd())
		ui = updated.(*UI)
		if !ui.chat.loading {
			t.Fatalf("expected loading to continue during multi-turn event %d", i)
		}
	}

	updated, listenCmd = ui.Update(listenCmd())
	ui = updated.(*UI)

	if ui.chat.loading {
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
	if len(ui.chat.messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(ui.chat.messages))
	}
	if textContent(ui.chat.messages[3].Parts) != "hello" {
		t.Fatalf("expected final assistant content, got %q", textContent(ui.chat.messages[3].Parts))
	}
}

func TestStreamFailureShowsErrorAndKeepsPartialAssistantMessage(t *testing.T) {
	svc := newStubAppService()
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

	ui := New(svc)
	ui, listenCmd := bootstrapRestoredSession(t, ui)

	ui.chat.composer.value = "hello"
	updated, sendCmd := ui.Update(enterKey())
	ui = updated.(*UI)
	_ = sendCmd()

	updated, _ = ui.Update(listenCmd())
	ui = updated.(*UI)

	if textContent(ui.chat.messages[1].Parts) != "par" {
		t.Fatalf("expected partial reply, got %q", textContent(ui.chat.messages[1].Parts))
	}

	updated, _ = ui.Update(sendMessageDoneMsg{err: errors.New("stream failed")})
	ui = updated.(*UI)
	if ui.chat.errMessage == "" {
		t.Fatal("expected error message")
	}
}

func TestQueryFailureEventStopsLoadingAndShowsError(t *testing.T) {
	svc := newStubAppService()
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

	ui := New(svc)
	ui, listenCmd := bootstrapRestoredSession(t, ui)

	ui.chat.loading = true
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

	updated, _ := ui.Update(listenCmd())
	ui = updated.(*UI)

	if ui.chat.loading {
		t.Fatal("expected loading to stop after query failure")
	}
	if ui.chat.errMessage != "query failed" {
		t.Fatalf("expected query failure error, got %q", ui.chat.errMessage)
	}
}

func TestQueryCompletedEventStopsLoading(t *testing.T) {
	svc := newStubAppService()
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

	ui := New(svc)
	ui, listenCmd := bootstrapRestoredSession(t, ui)

	ui.chat.loading = true
	svc.events <- app.BaseEvent{
		T: app.EventAgent,
		Payload: agent.QueryEvent{
			Status: agent.QueryStatusCompleted,
			State: agent.LoopState{
				Transition: "assistant_completed",
			},
		},
	}

	updated, _ := ui.Update(listenCmd())
	ui = updated.(*UI)

	if ui.chat.loading {
		t.Fatal("expected loading to stop after query completion")
	}
}

type stubAppService struct {
	events               chan app.Event
	eventsCalls          int
	ensureSessionFn      func(ctx context.Context) error
	runQueryFn           func(ctx context.Context, sessionID string, prompt string) error
	history              []message.Message
	lastHistorySessionID string
	lastSessionID        string
	lastPrompt           string
}

func newStubAppService() *stubAppService {
	return &stubAppService{events: make(chan app.Event, 16)}
}

func (s *stubAppService) EnsureActiveSession(ctx context.Context) error {
	if s.ensureSessionFn == nil {
		return nil
	}
	return s.ensureSessionFn(ctx)
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

func (s *stubAppService) Events() <-chan app.Event {
	s.eventsCalls++
	return s.events
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

func enterKey() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
}

func bootstrapRestoredSession(t *testing.T, ui *UI) (*UI, tea.Cmd) {
	t.Helper()

	bootstrapMsg := ui.Init()()
	updated, listenCmd := ui.Update(bootstrapMsg)
	ui = updated.(*UI)

	updated, historyCmd := ui.Update(listenCmd())
	ui = updated.(*UI)

	updated, listenCmd = ui.Update(historyCmd())
	return updated.(*UI), listenCmd
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
