package model

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

const slashMenuHeight = 7

type slashCommand struct {
	name        string
	description string
}

func (c slashCommand) FilterValue() string { return c.name + " " + c.description }
func (c slashCommand) Title() string       { return c.name }
func (c slashCommand) Description() string { return c.description }

type SlashMenu struct {
	list  list.Model
	open  bool
	items []slashCommand
}

func newSlashMenu(commands []slashCommand) SlashMenu {
	items := make([]list.Item, 0, len(commands))
	for _, command := range commands {
		items = append(items, command)
	}

	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(1)
	delegate.SetSpacing(0)
	delegate.ShowDescription = false

	model := list.New(items, delegate, defaultWidth, slashMenuHeight)
	model.Title = "Commands"
	model.SetShowFilter(false)
	model.SetShowHelp(false)
	model.SetShowStatusBar(false)
	model.SetShowPagination(false)
	model.DisableQuitKeybindings()

	return SlashMenu{
		list:  model,
		items: append([]slashCommand(nil), commands...),
	}
}

func (s *SlashMenu) Open(filter string, width int) {
	s.open = true
	s.list.SetSize(max(1, width), slashMenuHeight)
	s.list.ResetSelected()
	s.list.SetFilterText(filter)
}

func (s *SlashMenu) Close() {
	s.open = false
	s.list.ResetFilter()
}

func (s SlashMenu) IsOpen() bool {
	return s.open
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

func (s SlashMenu) View() string {
	if !s.open {
		return ""
	}
	return s.list.View()
}

func defaultSlashCommands() []slashCommand {
	return []slashCommand{
		{name: "/historySession", description: "show session history"},
		{name: "/permission", description: "change permission level"},
		{name: "/newSession", description: "start a new session"},
		{name: "/compact", description: "compact current context"},
	}
}
