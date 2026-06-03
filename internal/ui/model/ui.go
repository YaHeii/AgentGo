package model

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/agent"
	agentcontract "github.com/YaHeii/agentGo/internal/agent/contract"
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/lifecycle"
	messageevent "github.com/YaHeii/agentGo/internal/message"
	message "github.com/YaHeii/agentGo/internal/message/contract"
	"github.com/YaHeii/agentGo/internal/session"
	uv "github.com/charmbracelet/ultraviolet"
)

const (
	defaultWidth        = 80
	defaultHeight       = 24
	minInputHeight      = 1
	slashInputPrefix    = "/"
	defaultHeaderStatus = "ready"
)

type appEventMsg struct {
	event app.Event
}

type sendMessageDoneMsg struct {
	err error
}

type historyLoadedMsg struct {
	sessionID string
	messages  []message.Message
	err       error
}

type bootstrapDoneMsg struct {
	err error
}

type appService interface {
	ListHistory(ctx context.Context, sessionID string) ([]message.Message, error)
	RunQuery(ctx context.Context, sessionID string, prompt string) error
	Events() <-chan app.Event
}

type uiState string

const (
	uiStateOnboarding uiState = "onboarding"
	uiStateInitialize uiState = "initialize"
	uiStateLanding    uiState = "landing"
	uiStateChat       uiState = "chat"
)

type UI struct {
	app       appService
	state     uiState
	layout    uiLayout
	width     int
	height    int
	header    Header
	input     Input
	slashMenu SlashMenu
	chat      Chat
	sessionID string
	events    <-chan app.Event
}

func New(appSvc appService) *UI {
	var events <-chan app.Event
	if appSvc != nil {
		events = appSvc.Events()
	}
	return &UI{
		app:       appSvc,
		state:     uiStateChat,
		header:    newHeader(),
		input:     newInput(),
		slashMenu: newSlashMenu(defaultSlashCommands()),
		chat:      newChat(),
		events:    events,
	}
}

func (u *UI) Init() tea.Cmd {
	if lifecycle.State != nil {
		u.sessionID = strings.TrimSpace(lifecycle.State.SessionID)
	}
	return tea.Batch(
		u.input.Init(),
		u.nextAppEventCmd(),
	)
}

func (u *UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		u.width = msg.Width
		u.height = msg.Height
		u.recomputeLayout()
		return u, nil
	case sendMessageDoneMsg:
		if msg.err != nil {
			u.chat.loading = false
			u.setTransientStatus(msg.err.Error())
			return u, nil
		}
		return u, nil
	case historyLoadedMsg:
		if msg.sessionID != u.sessionID {
			return u, u.nextAppEventCmd()
		}
		if msg.err != nil {
			u.setTransientStatus(msg.err.Error())
			return u, u.nextAppEventCmd()
		}
		u.chat.SetMessages(msg.messages)
		u.chat.renderStableMarkdown(msg.messages)
		u.chat.refreshTranscript()
		return u, u.nextAppEventCmd()
	case appEventMsg:
		cmd := u.handleAppEvent(msg.event)
		if cmd != nil {
			return u, cmd
		}
		return u, u.nextAppEventCmd()
	case tea.KeyPressMsg:
		return u.handleKeyPress(msg)
	case tea.MouseMsg:
		return u.handleMouse(msg)
	}

	return u, nil
}

func (u *UI) View() tea.View {
	width := max(1, u.width)
	height := max(1, u.height)
	screen := uv.NewScreenBuffer(width, height)
	copyUI := *u
	copyUI.Draw(screen, screen.Bounds())

	view := tea.NewView(screen.Render())
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (u *UI) Draw(scr uv.Screen, area uv.Rectangle) {
	u.layout = generateLayout(area, u.header.Height(), u.input.Height(max(1, area.Dx())), u.slashMenu.IsOpen())
	clearScreen(scr, area)

	uv.NewStyledString(u.header.View(max(1, u.layout.header.Dx()))).Draw(scr, u.layout.header)
	if u.state == uiStateChat {
		u.chat.Draw(scr, u.layout.main)
	}
	uv.NewStyledString(u.input.View(max(1, u.layout.input.Dx()))).Draw(scr, u.layout.input)
	if u.slashMenu.IsOpen() && !u.layout.slashMenu.Empty() {
		uv.NewStyledString(u.slashMenu.View()).Draw(scr, u.layout.slashMenu)
	}
}

func (u *UI) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return u, tea.Quit
	}

	if u.slashMenu.IsOpen() {
		switch msg.Key().Code {
		case tea.KeyUp, tea.KeyDown:
			return u, u.slashMenu.HandleKey(msg)
		case tea.KeyEnter, tea.KeyKpEnter:
			return u.executeSelectedSlashCommand()
		case tea.KeyEscape:
			u.slashMenu.Close()
			u.recomputeLayout()
			return u, nil
		}
	}

	switch msg.Key().Code {
	case tea.KeyEnter, tea.KeyKpEnter:
		return u.sendInput()
	default:
		if msg.Key().Text == "" && !isInputEditingKey(msg) {
			return u, nil
		}
		updated, cmd := u.input.Update(msg)
		u.input = updated
		u.syncSlashMenuFromInput()
		u.recomputeLayout()
		return u, cmd
	}
}

func (u *UI) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	region, local := localMouse(u.layout, msg)
	switch region {
	case uiRegionMain:
		u.chat.HandleMouse(local)
	case uiRegionSlashMenu:
		cmd := u.slashMenu.HandleMouse(local)
		if click, ok := local.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft {
			return u.executeSelectedSlashCommandWithCmd(cmd)
		}
		return u, cmd
	}
	return u, nil
}

func (u *UI) handleAppEvent(event app.Event) tea.Cmd {
	switch event.Type() {
	case app.EventSession:
		sessionEvent, ok := event.Data().(session.SessionEvent)
		if !ok || sessionEvent.Session == nil {
			return nil
		}
		u.sessionID = sessionEvent.Session.ID
		if sessionEvent.Status == session.StatusRestored || sessionEvent.Status == session.StatusSwitched {
			return loadHistoryCmd(u.app, sessionEvent.Session.ID)
		}
	case app.EventMessage:
		messageEvent, ok := event.Data().(messageevent.MessageEvent)
		if !ok || messageEvent.Message == nil {
			return nil
		}
		u.chat.AppendOrUpsertMessage(*messageEvent.Message)
		u.chat.refreshTranscript()
	case app.EventAgent:
		agentEvent, ok := event.Data().(agent.QueryEvent)
		if !ok {
			return nil
		}
		u.chat.SetMessages(agentEvent.State.Messages)
		switch agentEvent.Status {
		case agentcontract.LoopStarted, agentcontract.LoopAwaitingTool:
			u.chat.clearMarkdownForMessages(u.chat.messages)
			u.chat.loading = true
			u.setTransientStatus("assistant is thinking...")
		case agentcontract.LoopCompleted:
			u.chat.renderTerminalMarkdown(u.chat.messages)
			u.chat.loading = false
			u.setTransientStatus(defaultHeaderStatus)
		case agentcontract.LoopFinishFailed:
			u.chat.renderTerminalMarkdown(u.chat.messages)
			u.chat.loading = false
			if agentEvent.Err != nil {
				u.setTransientStatus(agentEvent.Err.Error())
			}
		}
		u.chat.refreshTranscript()
	}
	return nil
}

func (u *UI) sendInput() (tea.Model, tea.Cmd) {
	if u.chat.loading {
		return u, nil
	}
	prompt := strings.TrimSpace(u.input.Value())
	if prompt == "" || u.sessionID == "" {
		return u, nil
	}

	u.input.Clear()
	u.slashMenu.Close()
	u.recomputeLayout()
	u.chat.loading = true
	u.setTransientStatus("assistant is thinking...")
	return u, runQueryCmd(u.app, u.sessionID, prompt)
}

func (u *UI) executeSelectedSlashCommand() (tea.Model, tea.Cmd) {
	return u.executeSelectedSlashCommandWithCmd(nil)
}

func (u *UI) executeSelectedSlashCommandWithCmd(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	command, ok := u.slashMenu.SelectedCommand()
	if !ok {
		return u, cmd
	}

	u.input.Clear()
	u.slashMenu.Close()
	u.setTransientStatus("not implemented: " + command)
	u.recomputeLayout()
	return u, cmd
}

func (u *UI) syncSlashMenuFromInput() {
	filter, ok := u.input.SlashFilter()
	if !ok {
		u.slashMenu.Close()
		return
	}
	u.slashMenu.Open(filter, max(1, u.width))
}

func (u *UI) localMouse(msg tea.MouseMsg) (uiRegion, tea.MouseMsg) {
	return localMouse(u.layout, msg)
}

func (u *UI) recomputeLayout() {
	u.layout = generateLayout(
		uv.Rect(0, 0, max(1, u.width), max(1, u.height)),
		u.header.Height(),
		u.input.Height(max(1, u.width)),
		u.slashMenu.IsOpen(),
	)

	mainWidth := max(1, u.layout.main.Dx())
	mainHeight := max(1, u.layout.main.Dy())
	u.chat.Resize(mainWidth, mainHeight)

	if u.slashMenu.IsOpen() {
		u.slashMenu.Resize(max(1, u.layout.slashMenu.Dx()), max(1, u.layout.slashMenu.Dy()))
	}
}

func (u *UI) setTransientStatus(status string) {
	u.header.SetTransientStatus(status)
}

func (u *UI) nextAppEventCmd() tea.Cmd {
	if u.events == nil {
		return nil
	}
	return waitAppEventCmd(u.events)
}

func clearScreen(scr uv.Screen, area uv.Rectangle) {
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			scr.SetCell(x, y, nil)
		}
	}
}

func isInputEditingKey(msg tea.KeyPressMsg) bool {
	switch msg.Key().Code {
	case tea.KeyBackspace, tea.KeyDelete, tea.KeyLeft, tea.KeyRight, tea.KeyUp, tea.KeyDown,
		tea.KeyHome, tea.KeyEnd, tea.KeyPgUp, tea.KeyPgDown:
		return true
	}
	return strings.HasPrefix(msg.String(), "alt+") || strings.HasPrefix(msg.String(), "ctrl+")
}
