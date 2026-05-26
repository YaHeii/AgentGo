package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/app"
	messageevent "github.com/YaHeii/agentGo/internal/message"
	message "github.com/YaHeii/agentGo/internal/message/contract"
	"github.com/YaHeii/agentGo/internal/session"
)

type rootModel struct {
	app        chatService
	events     <-chan app.Event
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
		events: appSvc.Events(),
		width:  defaultWidth,
		height: defaultHeight,
	}
}

func bootstrapSessionCmd(appSvc chatService) tea.Cmd {
	return func() tea.Msg {
		if err := appSvc.InitializePermissionLevel(context.Background()); err != nil {
			return bootstrapDoneMsg{err: err}
		}
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
		return m, waitAppEventCmd(m.events)
	case sendMessageDoneMsg:
		if msg.err != nil {
			m.errMessage = msg.err.Error()
		}
		return m, nil
	case appEventMsg:
		switch msg.event.Type() {
		case app.EventSession:
			if event, ok := msg.event.Data().(session.SessionEvent); ok && event.Session != nil {
				m.sessionID = event.Session.ID
			}
		case app.EventMessage:
			if event, ok := msg.event.Data().(messageevent.MessageEvent); ok && event.Message != nil {
				m.upsertMessage(*event.Message)
			}
		case app.EventAgent:
			if event, ok := msg.event.Data().(agent.QueryEvent); ok {
				m.messages = append([]message.Message(nil), event.State.Messages...)

				switch event.Status {
				case agent.QueryStatusStarted, agent.QueryStatusDelta:
					m.loading = true
				case agent.QueryStatusCompleted:
					m.loading = false
				case agent.QueryStatusFailed:
					if event.Err != nil {
						m.errMessage = event.Err.Error()
					}
					m.loading = false
				}
			}
		}
		return m, waitAppEventCmd(m.events)
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
			return m, runQueryCmd(m.app, m.sessionID, prompt)
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

// UI layer only sends the raw user prompt. Query assembly stays below app/agent.
func runQueryCmd(appSvc chatService, sessionID string, prompt string) tea.Cmd {
	return func() tea.Msg {
		err := appSvc.RunQuery(context.Background(), sessionID, prompt)
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
