package bus

type Bus[T any] interface {
	Publish(event T)
	Events() <-chan T
}

type InMemoryBus[T any] struct {
	events chan T
}

func NewBus[T any](buffer int) *InMemoryBus[T] {
	if buffer <= 0 {
		buffer = 1
	}

	return &InMemoryBus[T]{
		events: make(chan T, buffer),
	}
}

func (b *InMemoryBus[T]) Publish(event T) {
	b.events <- event
}

func (b *InMemoryBus[T]) Events() <-chan T {
	return b.events
}
