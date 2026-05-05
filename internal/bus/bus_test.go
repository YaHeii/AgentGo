package bus

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type testEvent struct {
	Name  string
	Value int
}

func TestInMemoryBusPublishesEventsInOrder(t *testing.T) {
	t.Parallel()

	b := NewBus[testEvent](4)

	first := testEvent{Name: "first", Value: 1}
	second := testEvent{Name: "second", Value: 2}

	b.Publish(first)
	b.Publish(second)

	require.Equal(t, first, <-b.Events())
	require.Equal(t, second, <-b.Events())
}

func TestInMemoryBusKeepsPayload(t *testing.T) {
	t.Parallel()

	b := NewBus[testEvent](1)
	want := testEvent{Name: "payload", Value: 42}

	b.Publish(want)

	require.Equal(t, want, <-b.Events())
}
