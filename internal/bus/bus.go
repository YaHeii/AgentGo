package bus

import (
	"context"
	"sync"
)
//XXX：sub could be a metadata(SubscriberInfo)
type Bus[T any] interface {
	Publish(event T)
	Subscribe(ctx context.Context) <-chan T
	Shutdown()
}

type InMemoryBus[T any] struct {
	mu     sync.RWMutex
	subs   map[chan T]struct{}
	buffer int
	done   chan struct{}
}

func NewBus[T any](buffer int) *InMemoryBus[T] {
	if buffer <= 0 {
		buffer = 1
	}

	return &InMemoryBus[T]{
		subs:   make(map[chan T]struct{}),
		buffer: buffer,
		done:   make(chan struct{}),
	}
}

func (b *InMemoryBus[T]) Publish(event T) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	select {
	case <-b.done:
		return
	default:
	}

	for sub := range b.subs {
		sub <- event //Blocked release
	}
}

func (b *InMemoryBus[T]) Subscribe(ctx context.Context) <-chan T {
	ch := make(chan T, b.buffer)

	b.mu.Lock()
	select {
	case <-b.done:
		close(ch)
		b.mu.Unlock()
		return ch
	default:
	}
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
		case <-b.done:
		}

		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}()

	return ch
}

func (b *InMemoryBus[T]) Shutdown() {
	select {
	case <-b.done:
		return
	default:
		close(b.done)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}
