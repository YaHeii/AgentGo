package provider

import (
	"context"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
)

type StreamingLLM interface {
	StreamChat(ctx context.Context, req Request) <-chan StreamEvent
}

type Request struct {
	SessionID string
}

type messageStore interface {
	ListMessages(ctx context.Context, sessionID string, d app.Dispatcher) ([]message.Message, error)
}
// Now we only use the openai provider
// To support more clients, 
// the API is being retained and is currently implemented in the provider package.
type streamClient interface {
	streamMessages(ctx context.Context, messages []message.Message) <-chan StreamEvent
}
