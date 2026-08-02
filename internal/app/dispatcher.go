package app

import (
	"context"
	"sync"
)

type Dispatcher interface {
	Dispatch(evt Event)
	Subscribe(ctx context.Context) <-chan Event
}

type appDispatcher struct {
	hub *memoryHub
}

func NewDispatcher(buffer int) Dispatcher {
	return &appDispatcher{
		hub: newMemoryHub(buffer),
	}
}

func (d *appDispatcher) Dispatch(evt Event) {
	if d == nil || d.hub == nil || evt == nil {
		return
	}

	route, ok := routeForEvent(evt)
	if !ok {
		return
	}
	if route.Delivery&DeliveryMemory == 0 {
		return
	}

	d.hub.publish(evt)
}

func (d *appDispatcher) Subscribe(ctx context.Context) <-chan Event {
	if d == nil || d.hub == nil {
		ch := make(chan Event)
		close(ch)
		return ch
	}
	return d.hub.subscribe(ctx)
}

type memoryHub struct {
	mu     sync.RWMutex
	subs   map[chan Event]struct{}
	buffer int
	done   chan struct{}
}

func newMemoryHub(buffer int) *memoryHub {
	if buffer <= 0 {
		buffer = 1
	}

	return &memoryHub{
		subs:   make(map[chan Event]struct{}),
		buffer: buffer,
		done:   make(chan struct{}),
	}
}

func (h *memoryHub) publish(evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	select {
	case <-h.done:
		return
	default:
	}

	for sub := range h.subs {
		sub <- evt
	}
}

func (h *memoryHub) subscribe(ctx context.Context) <-chan Event {
	ch := make(chan Event, h.buffer)

	h.mu.Lock()
	select {
	case <-h.done:
		close(ch)
		h.mu.Unlock()
		return ch
	default:
	}
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
		case <-h.done:
		}

		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}()

	return ch
}

func (h *memoryHub) shutdown() {
	select {
	case <-h.done:
		return
	default:
		close(h.done)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
}
