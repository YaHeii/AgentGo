package cmd

import (
	"context"
	"errors"
	"fmt"
	
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/YaHeii/agentGo/internal/provider"
	provideropenai "github.com/YaHeii/agentGo/internal/provider/openai"
	"github.com/YaHeii/agentGo/internal/utils"
)

const (
	roleUser      = "user"
	roleAssistant = "assistant"
	defaultWidth  = 80
	defaultHeight = 24
)

type chatResponseMsg struct {
	reply string
	err   error
}

type chatModel struct {
	llm        provider.LLM
	messages   []provider.Message
	input      string
	errMessage string
	loading    bool
	width      int
	height     int
}

func NewChatModel(llm provider.LLM) chatModel {
	return chatModel{
		llm:    llm,
		width:  defaultWidth,
		height: defaultHeight,
	}
}

func (m chatModel) Init() tea.Cmd {
	return nil
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			if m.loading {
				return m, nil
			}
			prompt := strings.TrimSpace(m.input)
			if prompt == "" {
				return m, nil
			}

			m.errMessage = ""
			m.loading = true
			m.input = ""
			m.messages = append(m.messages, provider.Message{
				Role:    roleUser,
				Content: prompt,
			})

			history := append([]provider.Message(nil), m.messages...)
			return m, requestReplyCmd(m.llm, history)
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
	case chatResponseMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}

		reply := strings.TrimSpace(msg.reply)
		if reply == "" {
			reply = "(empty response)"
		}
		m.messages = append(m.messages, provider.Message{
			Role:    roleAssistant,
			Content: reply,
		})
	}

	return m, nil
}

func (m chatModel) View() tea.View {
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

func requestReplyCmd(llm provider.LLM, messages []provider.Message) tea.Cmd {
	return func() tea.Msg {
		reply, err := llm.Chat(context.Background(), messages)
		return chatResponseMsg{
			reply: reply,
			err:   err,
		}
	}
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

func ProviderConfigFromAppConfig(cfg utils.Config) (provideropenai.Config, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return provideropenai.Config{}, errors.New("API_KEY is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return provideropenai.Config{}, errors.New("MODEL is required")
	}

	return provideropenai.Config{
		BaseURL: strings.TrimSpace(cfg.BaseURL),
		APIKey:  strings.TrimSpace(cfg.APIKey),
		Model:   strings.TrimSpace(cfg.Model),
	}, nil
}
