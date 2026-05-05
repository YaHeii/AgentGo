package message

import "github.com/YaHeii/agentGo/internal/store"

type SendMessageParams struct {
	SessionID string
	Prompt    string
}

type SendMessageResult struct {
	User      store.Message
	Assistant store.Message
}
