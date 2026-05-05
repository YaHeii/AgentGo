package provider

import "context"

type StreamEventType string

const (
	StreamEventDelta StreamEventType = "delta"
	StreamEventDone  StreamEventType = "done"
)

type StreamEvent struct {
	Type  StreamEventType
	Delta string
	Err   error
}

type StreamingLLM interface {
	StreamChat(ctx context.Context, messages []Message) <-chan StreamEvent
}
