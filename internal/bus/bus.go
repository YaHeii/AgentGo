package bus

import "github.com/YaHeii/agentGo/internal/event"

type EventBus interface {
	Publish(event event.Event)
	Events() <-chan event.Event
}

type InMemoryBus struct {
	events chan event.Event
}

var _ EventBus = (*InMemoryBus)(nil)

func NewBus(buffer int) *InMemoryBus {
	if buffer <= 0 {
		buffer = 1
	}

	return &InMemoryBus{
		events: make(chan event.Event, buffer),
	}
}

func (b *InMemoryBus) Publish(event event.Event) {
	b.events <- event
}

func (b *InMemoryBus) Events() <-chan event.Event {
	return b.events
}
