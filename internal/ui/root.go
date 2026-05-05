package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/app"
)

const (
	roleUser      = "user"
	roleAssistant = "assistant"
	defaultWidth  = 80
	defaultHeight = 24
)

type bootstrapDoneMsg struct {
	err error
}

type sendMessageDoneMsg struct {
	err error
}

type appEventMsg struct {
	event app.Event
}

type chatService interface {
	EnsureActiveSession(ctx context.Context) (app.Session, error)
	SendMessage(ctx context.Context, params app.SendMessageParams) (app.SendMessageResult, error)
	Events() <-chan app.Event
}

type rootModel struct {
	app        chatService
	events     <-chan app.Event
	sessionID  string
	messages   []app.Message
	input      string
	errMessage string
	loading    bool
	width      int
	height     int
}

func NewRootModel(appSvc chatService) rootModel {
	return rootModel{
		app:    appSvc,
		events: appSvc.Events(),
		width:  defaultWidth,
		height: defaultHeight,
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
		switch event := msg.event.(type) {
		case app.SessionReadyEvent:
			m.sessionID = event.Session.ID
		case app.ConversationHydratedEvent:
			m.messages = append([]app.Message(nil), event.Messages...)
		case app.MessageCreatedEvent:
			m.upsertMessage(event.Message)
		case app.MessageDeltaEvent:
			m.upsertMessage(event.Message)
		case app.MessageCompletedEvent:
			m.upsertMessage(event.Message)
			m.loading = false
		case app.MessageFailedEvent:
			m.upsertMessage(event.Message)
			if event.Err != nil {
				m.errMessage = event.Err.Error()
			}
			m.loading = false
		case app.MessageCancelledEvent:
			m.upsertMessage(event.Message)
			if event.Err != nil {
				m.errMessage = event.Err.Error()
			}
			m.loading = false
		}
		return m, waitAppEventCmd(m.events)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
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
		lines = append(lines, fmt.Sprintf("%s %s", roleLabel(msg.Role), msg.Content))
	}

	if m.errMessage != "" {
		lines = append(lines, "", "error: "+m.errMessage)
	}
	if m.loading {
		lines = append(lines, "", "assistant is thinking...")
	}

	lines = append(lines, "", "> "+m.input, "", "Enter to send. q or ctrl+c to quit.")

	maxLines := m.height
	if maxLines <= 0 {
		maxLines = defaultHeight
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}

	return tea.NewView(strings.Join(lines, "\n"))
}

func bootstrapSessionCmd(appSvc chatService) tea.Cmd {
	return func() tea.Msg {
		_, err := appSvc.EnsureActiveSession(context.Background())
		return bootstrapDoneMsg{
			err: err,
		}
	}
}

func sendMessageCmd(appSvc chatService, sessionID string, prompt string) tea.Cmd {
	return func() tea.Msg {
		_, err := appSvc.SendMessage(context.Background(), app.SendMessageParams{
			SessionID: sessionID,
			Prompt:    prompt,
		})
		return sendMessageDoneMsg{err: err}
	}
}

func waitAppEventCmd(events <-chan app.Event) tea.Cmd {
	return func() tea.Msg {
		event := <-events
		return appEventMsg{event: event}
	}
}

func (m *rootModel) upsertMessage(msg app.Message) {
	for i := range m.messages {
		if m.messages[i].ID == msg.ID {
			m.messages[i] = msg
			return
		}
	}

	m.messages = append(m.messages, msg)
}

func roleLabel(role string) string {
	switch role {
	case roleAssistant:
		return "ai:"
	default:
		return "you:"
	}
}

func isControlKey(text string) bool {
	for _, r := range text {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}
