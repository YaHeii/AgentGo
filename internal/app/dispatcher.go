package app

import (
	"context"

	"github.com/YaHeii/agentGo/internal/bus"
)

type Dispatcher interface {
	Dispatch(evt Event)
	Subscribe(ctx context.Context) <-chan Event
}

type appDispatcher struct {
	eventBus bus.Bus[Event]
}

func NewDispatcher(buffer int) Dispatcher {
	return &appDispatcher{
		eventBus: bus.NewBus[Event](buffer),
	}
}

func (d *appDispatcher) Dispatch(evt Event) {
	d.eventBus.Publish(evt)
}

func (d *appDispatcher) Subscribe(ctx context.Context) <-chan Event {
	return d.eventBus.Subscribe(ctx)
}
