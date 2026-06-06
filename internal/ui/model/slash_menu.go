package model

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/lifecycle"
	"github.com/YaHeii/agentGo/internal/session/contract"
)

const slashMenuHeight = 7

type slashMenuMode string

const (
	slashMenuModeCommands    slashMenuMode = "commands"
	slashMenuModeSessions    slashMenuMode = "sessions"
	slashMenuModePermissions slashMenuMode = "permissions"
)

type slashCommand struct {
	name        string
	description string
}

func (c slashCommand) FilterValue() string { return c.name + " " + c.description }
func (c slashCommand) Title() string       { return c.name }
func (c slashCommand) Description() string { return c.description }

type sessionItem struct {
	session contract.Session
}

func (i sessionItem) FilterValue() string { return i.session.Title + " " + i.session.ID }
func (i sessionItem) Title() string {
	if i.session.Title != "" {
		return i.session.Title
	}
	return i.session.ID
}
func (i sessionItem) Description() string { return i.session.ID }

type permissionItem struct {
	level lifecycle.PermissionLevel
}

func (i permissionItem) FilterValue() string { return permissionLabel(i.level) }
func (i permissionItem) Title() string       { return permissionLabel(i.level) }
func (i permissionItem) Description() string { return "" }

type SlashMenu struct {
	list     list.Model
	open     bool
	mode     slashMenuMode
	commands []slashCommand
}

func newSlashMenu(commands []slashCommand) SlashMenu {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(1)
	delegate.SetSpacing(0)
	delegate.ShowDescription = false

	model := list.New(commandItems(commands), delegate, defaultWidth, slashMenuHeight)
	model.Title = "Commands"
	model.SetShowFilter(false)
	model.SetShowHelp(false)
	model.SetShowStatusBar(false)
	model.SetShowPagination(false)
	model.DisableQuitKeybindings()

	return SlashMenu{
		list:     model,
		mode:     slashMenuModeCommands,
		commands: append([]slashCommand(nil), commands...),
	}
}

func (s *SlashMenu) Open(filter string, width int) {
	s.mode = slashMenuModeCommands
	s.open = true
	s.list.Title = "Commands"
	s.list.SetItems(commandItems(s.commands))
	s.list.SetSize(max(1, width), slashMenuHeight)
	s.list.ResetSelected()
	s.list.SetFilterText(filter)
}

func (s *SlashMenu) OpenSessions(sessions []contract.Session, width int) {
	items := make([]list.Item, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, sessionItem{session: session})
	}

	s.mode = slashMenuModeSessions
	s.open = true
	s.list.Title = "Sessions"
	s.list.SetItems(items)
	s.list.SetSize(max(1, width), slashMenuHeight)
	s.list.ResetSelected()
	s.list.ResetFilter()
}

func (s *SlashMenu) OpenPermissions(width int) {
	items := make([]list.Item, 0, 3)
	for _, level := range []lifecycle.PermissionLevel{
		lifecycle.SafeLevel,
		lifecycle.AttentionLevel,
		lifecycle.DangerLevel,
	} {
		items = append(items, permissionItem{level: level})
	}

	s.mode = slashMenuModePermissions
	s.open = true
	s.list.Title = "Permissions"
	s.list.SetItems(items)
	s.list.SetSize(max(1, width), slashMenuHeight)
	s.list.ResetSelected()
	s.list.ResetFilter()
}

func (s *SlashMenu) Close() {
	s.open = false
	s.mode = slashMenuModeCommands
	s.list.ResetFilter()
}

func (s SlashMenu) IsOpen() bool {
	return s.open
}

func (s SlashMenu) Mode() slashMenuMode {
	return s.mode
}

func (s *SlashMenu) Resize(width int, height int) {
	s.list.SetSize(max(1, width), max(1, height))
}

func (s *SlashMenu) HandleKey(msg tea.KeyPressMsg) tea.Cmd {
	updated, cmd := s.list.Update(msg)
	s.list = updated
	return cmd
}

func (s *SlashMenu) HandleMouse(msg tea.MouseMsg) tea.Cmd {
	updated, cmd := s.list.Update(msg)
	s.list = updated
	return cmd
}

func (s SlashMenu) SelectedCommand() (string, bool) {
	selected, ok := s.list.SelectedItem().(slashCommand)
	if !ok {
		return "", false
	}
	return selected.name, true
}

func (s SlashMenu) SelectedSessionID() (string, bool) {
	selected, ok := s.list.SelectedItem().(sessionItem)
	if !ok {
		return "", false
	}
	return selected.session.ID, true
}

func (s SlashMenu) SelectedPermissionLevel() (lifecycle.PermissionLevel, bool) {
	selected, ok := s.list.SelectedItem().(permissionItem)
	if !ok {
		return lifecycle.SafeLevel, false
	}
	return selected.level, true
}

func (s SlashMenu) View() string {
	if !s.open {
		return ""
	}
	return s.list.View()
}

func commandItems(commands []slashCommand) []list.Item {
	items := make([]list.Item, 0, len(commands))
	for _, command := range commands {
		items = append(items, command)
	}
	return items
}

func defaultSlashCommands() []slashCommand {
	return []slashCommand{
		{name: "/historySession", description: "show session history"},
		{name: "/permission", description: "change permission level"},
		{name: "/newSession", description: "start a new session"},
		{name: "/compact", description: "compact current context"},
	}
}
