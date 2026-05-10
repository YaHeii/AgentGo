package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/session"
)

type rootModel struct {
	app        chatService
	sessionID  string
	messages   []message.Message
	input      string
	errMessage string
	loading    bool
	width      int
	height     int
}

type bootstrapDoneMsg struct {
	err error
}

func NewRootModel(appSvc chatService) rootModel {
	return rootModel{
		app:    appSvc,
		width:  defaultWidth,
		height: defaultHeight,
	}
}

// TODO: add some Pre-security checks
// At here, can mock cc approach,
// The purpose of delaying the rendering until after the initial rendering
// is to avoid blocking the appearance of the terminal interface.
// While the user sees the prompt and begins to think and type,
// these background tasks are already being completed in parallel.
// src/main.tsx:388-431
//
//	export function startDeferredPrefetches(): void {
//	  // This function runs after first render, so it doesn't block the initial paint.
//	  void initUser();
//	  void getUserContext();
//	  prefetchSystemContextIfSafe();
//	  void getRelevantTips();
//	  void countFilesRoundedRg(getCwd(), AbortSignal.timeout(3000), []);
//	  void initializeAnalyticsGates();
//	  void refreshModelCapabilities();
//	  void settingsChangeDetector.initialize();
//	  // ...
//	}
func bootstrapSessionCmd(appSvc chatService) tea.Cmd {
	return func() tea.Msg {
		err := appSvc.EnsureActiveSession(context.Background())
		return bootstrapDoneMsg{
			err: err,
		}
	}
}

func (m rootModel) Init() tea.Cmd {
	return bootstrapSessionCmd(m.app)
}

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case bootstrapDoneMsg:
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		return m, waitAppEventCmd(m.app.Events())
	case sendMessageDoneMsg:
		if msg.err != nil {
			m.errMessage = msg.err.Error()
		}
		return m, nil
	case appEventMsg:
		switch event := msg.event.(type) {
		case session.SessionRestoredEvent:
			m.sessionID = event.Session.ID
			m.messages = append([]message.Message(nil), event.Messages...)
		case session.SessionSwitchedEvent:
			m.sessionID = event.SessionID
		case message.MessageCreatedEvent:
			m.upsertMessage(event.Message)
		case message.MessageDeltaEvent:
			m.upsertMessage(event.Message)
		case message.MessageCompletedEvent:
			m.upsertMessage(event.Message)
			m.loading = false
		case message.MessageFailedEvent:
			m.upsertMessage(event.Message)
			if event.Err != nil {
				m.errMessage = event.Err.Error()
			}
			m.loading = false
		case message.MessageCancelledEvent:
			m.upsertMessage(event.Message)
			if event.Err != nil {
				m.errMessage = event.Err.Error()
			}
			m.loading = false
		case agent.QueryCompletedEvent:
			m.loading = false
		case agent.QueryFailedEvent:
			if event.Err != nil {
				m.errMessage = event.Err.Error()
			}
			m.loading = false
		}
		return m, waitAppEventCmd(m.app.Events())
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			if m.loading {
				return m, nil
			}
			prompt := strings.TrimSpace(m.input)
			if prompt == "" || m.sessionID == "" {
				return m, nil
			}

			m.errMessage = ""
			m.loading = true
			m.input = ""
			return m, sendMessageCmd(m.app, m.sessionID, prompt)
		case "backspace":
			if len(m.input) > 0 {
				runes := []rune(m.input)
				m.input = string(runes[:len(runes)-1])
			}
		default:
			if !m.loading && msg.Key().Text != "" && !isControlKey(msg.Key().Text) {
				m.input += msg.Key().Text
			}
		}
	}

	return m, nil
}

func (m rootModel) View() tea.View {
	lines := make([]string, 0, len(m.messages)*2+6)
	lines = append(lines, "agentGo", "")

	for _, msg := range m.messages {
		lines = append(lines, fmt.Sprintf("%s %s", kindLabel(msg.Kind), textContent(msg.Parts)))
	}

	if m.errMessage != "" {
		lines = append(lines, "", "error: "+m.errMessage)
	}
	if m.loading {
		lines = append(lines, "", "assistant is thinking...")
	}

	lines = append(lines, "", "> "+m.input, "", "Enter to send. ctrl+c to quit.")

	maxLines := m.height
	if maxLines <= 0 {
		maxLines = defaultHeight
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}

	return tea.NewView(strings.Join(lines, "\n"))
}

// UIlayer should only send the basic prompt, tool schema .etc will be assembled in app layer
func sendMessageCmd(appSvc chatService, sessionID string, prompt string) tea.Cmd {
	return func() tea.Msg {
		err := appSvc.SendMessage(context.Background(), sessionID, prompt)
		return sendMessageDoneMsg{err: err}
	}
}

func waitAppEventCmd(events <-chan app.Event) tea.Cmd {
	return func() tea.Msg {
		event := <-events
		return appEventMsg{event: event}
	}
}

func (m *rootModel) upsertMessage(msg message.Message) {
	for i := range m.messages {
		if m.messages[i].ID == msg.ID {
			m.messages[i] = msg
			return
		}
	}

	m.messages = append(m.messages, msg)
}

func kindLabel(kind message.Kind) string {
	switch kind {
	case message.KindAssistant:
		return "ai:"
	default:
		return "you:"
	}
}

func textContent(parts []message.Part) string {
	for _, part := range parts {
		if part.Type == message.PartTypeText {
			return part.Text
		}
	}
	return ""
}

func isControlKey(text string) bool {
	for _, r := range text {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}
