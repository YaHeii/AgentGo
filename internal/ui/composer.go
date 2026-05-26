package ui

import tea "charm.land/bubbletea/v2"

type composerModel struct {
	value string
}

func newComposerModel() composerModel {
	return composerModel{}
}

func (m composerModel) Update(msg tea.Msg) composerModel {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m
	}

	switch keyMsg.String() {
	case "backspace":
		if len(m.value) == 0 {
			return m
		}
		runes := []rune(m.value)
		m.value = string(runes[:len(runes)-1])
		return m
	case "enter":
		return m
	default:
		if keyMsg.Key().Text != "" && !isControlKey(keyMsg.Key().Text) {
			m.value += keyMsg.Key().Text
		}
		return m
	}
}

func (m composerModel) Clear() composerModel {
	m.value = ""
	return m
}

func (m composerModel) View() string {
	return "> " + m.value
}
