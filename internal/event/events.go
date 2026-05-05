package event

import "github.com/YaHeii/agentGo/internal/store"

type Event interface {
	isAppEvent()
}

type SessionReadyEvent struct {
	Session store.Session
}

func (SessionReadyEvent) isAppEvent() {}

type ConversationHydratedEvent struct {
	SessionID string
	Messages  []store.Message
}

func (ConversationHydratedEvent) isAppEvent() {}

type MessageCreatedEvent struct {
	Message store.Message
}

func (MessageCreatedEvent) isAppEvent() {}

type MessageDeltaEvent struct {
	Message store.Message
	Delta   string
}

func (MessageDeltaEvent) isAppEvent() {}

type MessageCompletedEvent struct {
	Message store.Message
}

func (MessageCompletedEvent) isAppEvent() {}

type MessageFailedEvent struct {
	Message store.Message
	Err     error
}

func (MessageFailedEvent) isAppEvent() {}

type MessageCancelledEvent struct {
	Message store.Message
	Err     error
}

func (MessageCancelledEvent) isAppEvent() {}
