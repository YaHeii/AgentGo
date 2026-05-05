package bus

import (
	"testing"

	"github.com/YaHeii/agentGo/internal/event"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/stretchr/testify/require"
)

func TestInMemoryBusPublishesEventsInOrder(t *testing.T) {
	t.Parallel()

	bus := NewBus(4)

	first := event.SessionReadyEvent{
		Session: store.Session{ID: "session-1"},
	}
	second := event.MessageDeltaEvent{
		Message: store.Message{ID: "assistant-1", SessionID: "session-1", Content: "hel"},
		Delta:   "hel",
	}

	bus.Publish(first)
	bus.Publish(second)

	gotFirst := <-bus.Events()
	gotSecond := <-bus.Events()

	require.IsType(t, event.SessionReadyEvent{}, gotFirst)
	require.IsType(t, event.MessageDeltaEvent{}, gotSecond)
	require.Equal(t, "session-1", gotFirst.(event.SessionReadyEvent).Session.ID)
	require.Equal(t, "hel", gotSecond.(event.MessageDeltaEvent).Delta)
}

func TestInMemoryBusKeepsMessagePayload(t *testing.T) {
	t.Parallel()

	bus := NewBus(1)

	want := event.MessageCompletedEvent{
		Message: store.Message{
			ID:        "assistant-1",
			SessionID: "session-1",
			Role:      "assistant",
			Content:   "hello",
			Status:    store.MessageStatusComplete,
		},
	}

	bus.Publish(want)

	got := (<-bus.Events()).(event.MessageCompletedEvent)
	require.Equal(t, want.Message.ID, got.Message.ID)
	require.Equal(t, want.Message.Content, got.Message.Content)
	require.Equal(t, want.Message.Status, got.Message.Status)
}
