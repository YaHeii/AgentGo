package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewEventUsesCatalogMetadata(t *testing.T) {
	t.Parallel()

	evt := NewEvent(EventSessionCreated, "created")

	require.Equal(t, EventSession, evt.Type())
	require.Equal(t, EventSessionCreated, evt.Name())
	require.Equal(t, "created", evt.Data())
}

func TestEventCatalogRoutesHaveRoutingKeys(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, eventCatalog)
	for name, meta := range eventCatalog {
		require.NotEmpty(t, meta.Type, "event %s must have an event type", name)
		require.NotEmpty(t, meta.Route.RoutingKey, "event %s must have a routing key", name)
		require.NotZero(t, meta.Route.Delivery, "event %s must have a delivery policy", name)
	}
}

func TestDispatcherPublishesMemoryRoutedEventsToSubscribers(t *testing.T) {
	t.Parallel()

	dispatcher := NewDispatcher(4)
	events := dispatcher.Subscribe(context.Background())

	want := NewEvent(EventSessionCreated, "created")
	dispatcher.Dispatch(want)

	require.Equal(t, want, <-events)
}

func TestDispatcherDoesNotReplayHistory(t *testing.T) {
	t.Parallel()

	dispatcher := NewDispatcher(4)
	dispatcher.Dispatch(NewEvent(EventSessionCreated, "before-subscribe"))

	events := dispatcher.Subscribe(context.Background())

	select {
	case got := <-events:
		t.Fatalf("unexpected replayed event: %#v", got)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestDispatcherClosesSubscriberWhenContextIsCancelled(t *testing.T) {
	t.Parallel()

	dispatcher := NewDispatcher(4)
	ctx, cancel := context.WithCancel(context.Background())
	events := dispatcher.Subscribe(ctx)

	cancel()

	select {
	case _, ok := <-events:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("subscriber was not closed after context cancellation")
	}
}

func TestDispatcherIgnoresTypeOnlyEventsWithoutName(t *testing.T) {
	t.Parallel()

	dispatcher := NewDispatcher(4)
	events := dispatcher.Subscribe(context.Background())

	want := BaseEvent{
		T:       EventSession,
		Payload: "legacy",
	}
	dispatcher.Dispatch(want)

	select {
	case got := <-events:
		t.Fatalf("unexpected event delivered: %#v", got)
	case <-time.After(20 * time.Millisecond):
	}
}
