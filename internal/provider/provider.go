package provider

import "context"

// Message is the provider-agnostic chat message format.
type Message struct {
	Role    string
	Content string
}

// LLM is the common interface implemented by concrete model providers.
type LLM interface {
	Chat(ctx context.Context, messages []Message) (string, error)
}
