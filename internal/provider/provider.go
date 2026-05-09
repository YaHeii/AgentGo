package provider

import "context"

// Message is the provider-agnostic chat message format.
type Message struct {
	Role    string
	Content string
}

// Request contains the provider-neutral inputs required to start one streamed turn.
type Request struct {
	Messages []Message

	// TODO: Add system/developer prompt layering when prompt ownership is settled.
	// TODO: Add tools and tool choice when ToolExecutor is introduced.
	// TODO: Add stream policy knobs once agent-side token policy is stable.
}

// LLM is the common interface implemented by concrete model providers.
type LLM interface {
	Chat(ctx context.Context, messages []Message) (string, error)
}
