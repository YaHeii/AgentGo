package app

type Event interface {
	isAppEvent()
}

type SessionReadyEvent struct {
	Session Session
}

func (SessionReadyEvent) isAppEvent() {}

type ConversationHydratedEvent struct {
	SessionID string
	Messages  []Message
}

func (ConversationHydratedEvent) isAppEvent() {}

type MessageCreatedEvent struct {
	Message Message
}

func (MessageCreatedEvent) isAppEvent() {}

type MessageDeltaEvent struct {
	Message Message
	Delta   string
}

func (MessageDeltaEvent) isAppEvent() {}

type MessageCompletedEvent struct {
	Message Message
}

func (MessageCompletedEvent) isAppEvent() {}

type MessageFailedEvent struct {
	Message Message
	Err     error
}

func (MessageFailedEvent) isAppEvent() {}

type MessageCancelledEvent struct {
	Message Message
	Err     error
}

func (MessageCancelledEvent) isAppEvent() {}
