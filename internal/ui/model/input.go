package model

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

const inputPrefix = "> "

type Input struct {
	model textarea.Model
	width int
}

func newInput() Input {
	model := textarea.New()
	model.Prompt = inputPrefix
	model.ShowLineNumbers = false
	model.Placeholder = ""
	model.CharLimit = 0
	model.MaxHeight = 0
	model.MaxContentHeight = 0
	model.DynamicHeight = true
	model.MinHeight = minInputHeight
	model.SetWidth(defaultWidth)
	model.SetValue("")
	model.KeyMap.InsertNewline.SetEnabled(false)
	model.Focus()

	input := Input{
		model: model,
		width: defaultWidth,
	}
	input.applyPrompt()
	return input
}

func (i Input) Init() tea.Cmd {
	return textarea.Blink
}

func (i *Input) Update(msg tea.Msg) (Input, tea.Cmd) {
	model, cmd := i.model.Update(msg)
	i.model = model
	i.applyPrompt()
	i.syncSize()
	return *i, cmd
}

func (i *Input) Append(text string) {
	if text == "" {
		return
	}
	i.model.InsertString(text)
	i.applyPrompt()
	i.syncSize()
}

func (i *Input) Backspace() {
	if i.Value() == "" {
		return
	}
	updated, _ := i.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	*i = updated
}

func (i *Input) Clear() {
	i.model.Reset()
	i.applyPrompt()
	i.syncSize()
}

func (i Input) Value() string {
	return i.model.Value()
}

func (i *Input) Height(width int) int {
	i.setWidth(width)
	return max(minInputHeight, i.model.Height())
}

func (i *Input) View(width int) string {
	i.setWidth(width)
	return i.model.View()
}

func (i Input) SlashFilter() (string, bool) {
	if !strings.HasPrefix(i.Value(), slashInputPrefix) {
		return "", false
	}
	return strings.TrimPrefix(i.Value(), slashInputPrefix), true
}

func (i *Input) setWidth(width int) {
	width = max(1, width)
	if i.width == width {
		return
	}
	i.width = width
	i.syncSize()
}

func (i *Input) syncSize() {
	i.model.SetWidth(max(1, i.width))
}

func (i *Input) applyPrompt() {
	if strings.HasPrefix(i.model.Value(), slashInputPrefix) {
		i.model.Prompt = ""
	} else {
		i.model.Prompt = inputPrefix
	}
}
