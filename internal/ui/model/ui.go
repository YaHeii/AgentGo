package model

import (
	"context"
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/app"
	messageevent "github.com/YaHeii/agentGo/internal/message"
	message "github.com/YaHeii/agentGo/internal/message/contract"
	"github.com/YaHeii/agentGo/internal/session"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/ultraviolet/layout"
)

const (
	defaultWidth  = 80
	defaultHeight = 24
)

type appEventMsg struct {
	event app.Event
}

// HACKER
type sendMessageDoneMsg struct {
	err error
}

// HACKER
type historyLoadedMsg struct {
	sessionID string
	messages  []message.Message
	err       error
}

type bootstrapDoneMsg struct {
	err error
}

type appService interface {
	EnsureActiveSession(ctx context.Context) error
	ListHistory(ctx context.Context, sessionID string) ([]message.Message, error)
	RunQuery(ctx context.Context, sessionID string, prompt string) error
	Events() <-chan app.Event
}



type UI struct {
	app    appService
	state  uiState
	focus  uiFocusState
	layout uiLayout
	width  int
	height int
	chat   Chat
}

type uiState string

const (
	uiStateOnboarding uiState = "onboarding"
	uiStateInitialize uiState = "initialize"
	uiStateLanding    uiState = "landing"
	uiStateChat       uiState = "chat"
)

type uiFocusState string

const (
	uiFocusEditor uiFocusState = "editor"
	uiFocusMain   uiFocusState = "main"
	uiFocusNone   uiFocusState = "none"
)

type uiLayout struct {
	// Displays basic information, including the logo, path, and permissions.
	header uv.Rectangle
	// Display the message list
	main uv.Rectangle
	// Disaplay the program status
	status uv.Rectangle
	// the input area
	editor uv.Rectangle
	// Use the / command to wake up.
	slashMenu uv.Rectangle
}

func New(appSvc appService) *UI {
	return &UI{
		app:   appSvc,
		state: uiStateChat,
		focus: uiFocusEditor,
		chat:  newChat(appSvc),
	}
}

func (u *UI) Init() tea.Cmd {
	return bootstrapSessionCmd(u.app)
}

func (u *UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		u.width = msg.Width
		u.height = msg.Height
		u.layout = u.generateLayout(uv.Rect(0, 0, msg.Width, msg.Height))
		u.chat.updateSize(msg.Width)
		u.chat.transcript = u.chat.transcript.updateSize(u.layout.main.Dx(), u.layout.main.Dy())
	case bootstrapDoneMsg:
		if msg.err != nil {
			u.chat.errMessage = msg.err.Error()
			u.chat.status = u.chat.status.setMessage(u.chat.errMessage)
			return u, nil
		}
		return u, waitAppEventCmd(u.chat.events)
	case sendMessageDoneMsg:
		if msg.err != nil {
			u.chat.errMessage = msg.err.Error()
			u.chat.status = u.chat.status.setMessage(u.chat.errMessage)
		}
		return u, nil
	case historyLoadedMsg:
		if msg.sessionID != u.chat.sessionID {
			return u, waitAppEventCmd(u.chat.events)
		}
		if msg.err != nil {
			u.chat.errMessage = msg.err.Error()
			u.chat.status = u.chat.status.setMessage(u.chat.errMessage)
			return u, waitAppEventCmd(u.chat.events)
		}
		u.chat.messages = append([]message.Message(nil), msg.messages...)
		u.chat.renderStableMarkdown(u.chat.messages)
		u.chat.refreshTranscript()
		return u, waitAppEventCmd(u.chat.events)
	case appEventMsg:
		switch msg.event.Type() {
		case app.EventSession:
			if event, ok := msg.event.Data().(session.SessionEvent); ok {
				if cmd := u.chat.handleSessionEvent(event); cmd != nil {
					return u, cmd
				}
			}
		case app.EventMessage:
			if event, ok := msg.event.Data().(messageevent.MessageEvent); ok {
				u.chat.handleMessageEvent(event)
			}
		case app.EventAgent:
			if event, ok := msg.event.Data().(agent.QueryEvent); ok {
				u.chat.handleAgentEvent(event)
			}
		}
		return u, waitAppEventCmd(u.chat.events)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return u, tea.Quit
		case "enter":
			if item, ok := u.chat.transcript.selectedItem(); ok && itemCanExpand(item) {
				u.chat.transcript = u.chat.transcript.Update(msg)
				return u, nil
			}
			if u.chat.loading {
				return u, nil
			}
			prompt := strings.TrimSpace(u.chat.composer.value)
			if prompt == "" || u.chat.sessionID == "" {
				return u, nil
			}

			u.chat.errMessage = ""
			u.chat.loading = true
			u.chat.status = u.chat.status.setMessage("assistant is thinking...")
			u.chat.composer = u.chat.composer.Clear()
			return u, runQueryCmd(u.app, u.chat.sessionID, prompt)
		default:
			if u.focus == uiFocusMain {
				u.chat.transcript = u.chat.transcript.Update(msg)
				return u, nil
			}
			u.chat.transcript = u.chat.transcript.Update(msg)
			if !u.chat.loading && u.focus == uiFocusEditor {
				u.chat.composer = u.chat.composer.Update(msg)
			}
		}
	}

	return u, nil
}

func (u *UI) View() tea.View {
	width := max(1, u.width)
	height := max(1, u.height)
	screen := uv.NewScreenBuffer(width, height)
	copyUI := *u
	copyUI.Draw(screen, screen.Bounds())
	return tea.NewView(screen.Render())
}

func (u *UI) Draw(scr uv.Screen, area uv.Rectangle) {
	layout := u.generateLayout(area)
	clearScreen(scr, area)

	headerText := "agentGo\nj/k to scroll. Enter to send. ctrl+c to quit."
	uv.NewStyledString(headerText).Draw(scr, layout.header)

	if u.state == uiStateChat {
		u.chat.Draw(scr, layout.main)
	}

	u.chat.DrawStatus(scr, layout.status)
	u.chat.DrawEditor(scr, layout.editor)
}

func (u *UI) generateLayout(area uv.Rectangle) uiLayout {
	if area.Empty() {
		return uiLayout{}
	}

	var body uv.Rectangle
	var editor uv.Rectangle
	layout.Vertical(
		layout.Len(max(1, area.Dy()-3)),
		layout.Fill(1),
	).Split(area).Assign(&body, &editor)

	var header uv.Rectangle
	var statusMain uv.Rectangle
	layout.Vertical(
		layout.Len(2),
		layout.Fill(1),
	).Split(body).Assign(&header, &statusMain)

	var main uv.Rectangle
	var status uv.Rectangle
	layout.Vertical(
		layout.Fill(1),
		layout.Len(1),
	).Split(statusMain).Assign(&main, &status)

	slashMenu := image.Rect(editor.Min.X, editor.Min.Y, editor.Max.X, editor.Min.Y)
	if u.state != uiStateChat {
		editor = image.Rect(editor.Min.X, editor.Min.Y, editor.Max.X, editor.Min.Y)
	}

	return uiLayout{
		header:    header,
		main:      main,
		status:    status,
		editor:    editor,
		slashMenu: slashMenu,
	}
}

func clearScreen(scr uv.Screen, area uv.Rectangle) {
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			scr.SetCell(x, y, nil)
		}
	}
}
