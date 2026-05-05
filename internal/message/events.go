package message

type Event interface {
	isMessageEvent()
}

type MessageCreatedEvent struct {
	Message Message
}

func (MessageCreatedEvent) isMessageEvent() {}

type MessageDeltaEvent struct {
	Message Message
	Delta   string
}

func (MessageDeltaEvent) isMessageEvent() {}

type MessageCompletedEvent struct {
	Message Message
}

func (MessageCompletedEvent) isMessageEvent() {}

type MessageFailedEvent struct {
	Message Message
	Err     error
}

func (MessageFailedEvent) isMessageEvent() {}

type MessageCancelledEvent struct {
	Message Message
	Err     error
}

func (MessageCancelledEvent) isMessageEvent() {}
