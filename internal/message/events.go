package message

import "github.com/YaHeii/agentGo/internal/app"

type Event interface {
	app.Event
	isMessageEvent()
}

type MessageCreatedEvent struct {
	Message Message
}

func (MessageCreatedEvent) isMessageEvent() {}

func (MessageCreatedEvent) Type() app.EventType { return app.EventTypeMessage }

type MessageDeltaEvent struct {
	Message Message
	Delta   string
}

func (MessageDeltaEvent) isMessageEvent() {}

func (MessageDeltaEvent) Type() app.EventType { return app.EventTypeMessage }

type MessageCompletedEvent struct {
	Message Message
}

func (MessageCompletedEvent) isMessageEvent() {}

func (MessageCompletedEvent) Type() app.EventType { return app.EventTypeMessage }

type MessageFailedEvent struct {
	Message Message
	Err     error
}

func (MessageFailedEvent) isMessageEvent() {}

func (MessageFailedEvent) Type() app.EventType { return app.EventTypeMessage }

type MessageCancelledEvent struct {
	Message Message
	Err     error
}

func (MessageCancelledEvent) isMessageEvent() {}

func (MessageCancelledEvent) Type() app.EventType { return app.EventTypeMessage }
