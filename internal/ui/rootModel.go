package ui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/app"
	messageevent "github.com/YaHeii/agentGo/internal/message"
	message "github.com/YaHeii/agentGo/internal/message/contract"
	"github.com/YaHeii/agentGo/internal/session"
	"github.com/charmbracelet/lipgloss"
)

type rootModel struct {
	app        chatService
	events     <-chan app.Event
	sessionID  string
	messages   []message.Message
	items      []transcriptItem
	markdown   map[string]string
	renderer   markdownRenderer
	errMessage string
	loading    bool
	width      int
	height     int
	transcript transcriptModel
	composer   composerModel
	status     statusModel
}

type bootstrapDoneMsg struct {
	err error
}

func NewRootModel(appSvc chatService) rootModel {
	return rootModel{
		app:        appSvc,
		events:     appSvc.Events(),
		width:      defaultWidth,
		height:     defaultHeight,
		markdown:   make(map[string]string),
		renderer:   newDefaultMarkdownRenderer(),
		transcript: newTranscriptModel(defaultWidth, defaultHeight),
		composer:   newComposerModel(),
		status:     newStatusModel(),
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
		m.transcript = m.transcript.updateSize(msg.Width, msg.Height)
	case bootstrapDoneMsg:
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			m.status = m.status.setMessage(m.errMessage)
			return m, nil
		}
		return m, waitAppEventCmd(m.events)
	case sendMessageDoneMsg:
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			m.status = m.status.setMessage(m.errMessage)
		}
		return m, nil
	case historyLoadedMsg:
		if msg.sessionID != m.sessionID {
			return m, waitAppEventCmd(m.events)
		}
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			m.status = m.status.setMessage(m.errMessage)
			return m, waitAppEventCmd(m.events)
		}
		m.messages = append([]message.Message(nil), msg.messages...)
		m.refreshTranscript()
		return m, waitAppEventCmd(m.events)
	case appEventMsg:
		switch msg.event.Type() {
		case app.EventSession:
			if event, ok := msg.event.Data().(session.SessionEvent); ok && event.Session != nil {
				m.sessionID = event.Session.ID
				if event.Status == session.StatusRestored || event.Status == session.StatusSwitched {
					return m, loadHistoryCmd(m.app, event.Session.ID)
				}
			}
		case app.EventMessage:
			if event, ok := msg.event.Data().(messageevent.MessageEvent); ok && event.Message != nil {
				m.upsertMessage(*event.Message)
				m.refreshTranscript()
			}
		case app.EventAgent:
			if event, ok := msg.event.Data().(agent.QueryEvent); ok {
				m.messages = append([]message.Message(nil), event.State.Messages...)

				switch event.Status {
				case agent.QueryStatusStarted, agent.QueryStatusDelta:
					m.clearMarkdownForMessages(m.messages)
					m.loading = true
					m.status = m.status.setMessage("assistant is thinking...")
				case agent.QueryStatusCompleted:
					m.renderTerminalMarkdown(m.messages)
					m.loading = false
					m.status = m.status.setMessage("")
				case agent.QueryStatusFailed:
					m.renderTerminalMarkdown(m.messages)
					if event.Err != nil {
						m.errMessage = event.Err.Error()
						m.status = m.status.setMessage(m.errMessage)
					}
					m.loading = false
				}
				m.refreshTranscript()
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
			prompt := strings.TrimSpace(m.composer.value)
			if prompt == "" || m.sessionID == "" {
				return m, nil
			}

			m.errMessage = ""
			m.loading = true
			m.status = m.status.setMessage("assistant is thinking...")
			m.composer = m.composer.Clear()
			return m, runQueryCmd(m.app, m.sessionID, prompt)
		default:
			m.transcript = m.transcript.Update(msg)
			if !m.loading {
				m.composer = m.composer.Update(msg)
			}
		}
	}

	return m, nil
}

func (m rootModel) View() tea.View {
	totalHeight := m.height
	if totalHeight <= 0 {
		totalHeight = defaultHeight
	}

	headerHeight := 2
	statusHeight := 1
	composerHeight := 3
	transcriptHeight := totalHeight - headerHeight - statusHeight - composerHeight
	if transcriptHeight < 3 {
		transcriptHeight = 3
	}

	transcript := m.transcript.updateSize(m.width, transcriptHeight)

	header := lipgloss.NewStyle().
		Bold(true).
		Width(m.width).
		Render(strings.Join([]string{"agentGo", "Enter to send. ctrl+c to quit."}, "\n"))

	statusText := m.status.View()
	if statusText == "" {
		statusText = "ready"
	}
	status := lipgloss.NewStyle().
		Width(m.width).
		Render(statusText)

	composer := lipgloss.NewStyle().
		Width(m.width).
		Height(composerHeight).
		Render(m.composer.View())

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		transcript.View(),
		status,
		composer,
	)

	return tea.NewView(content)
}

// UI layer only sends the raw user prompt. Query assembly stays below app/agent.
func runQueryCmd(appSvc chatService, sessionID string, prompt string) tea.Cmd {
	return func() tea.Msg {
		err := appSvc.RunQuery(context.Background(), sessionID, prompt)
		return sendMessageDoneMsg{err: err}
	}
}

func loadHistoryCmd(appSvc chatService, sessionID string) tea.Cmd {
	return func() tea.Msg {
		messages, err := appSvc.ListHistory(context.Background(), sessionID)
		return historyLoadedMsg{
			sessionID: sessionID,
			messages:  messages,
			err:       err,
		}
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

func (m *rootModel) refreshTranscript() {
	m.items = buildTranscriptItems(m.messages, m.markdown, m.transcript.expanded)
	m.transcript = m.transcript.setItems(m.items)
}

func (m *rootModel) clearMarkdownForMessages(messages []message.Message) {
	for _, msg := range messages {
		delete(m.markdown, msg.ID)
	}
}

func (m *rootModel) renderTerminalMarkdown(messages []message.Message) {
	if m.renderer == nil {
		return
	}
	for _, msg := range messages {
		if msg.Kind != message.KindAssistant {
			continue
		}
		text := textContent(msg.Parts)
		if strings.TrimSpace(text) == "" {
			continue
		}
		rendered, err := m.renderer.Render(text)
		if err != nil {
			continue
		}
		m.markdown[msg.ID] = rendered
	}
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
