package bus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type testEvent struct {
	Name  string
	Value int
}

func TestInMemoryBusPublishesEventsToAllSubscribers(t *testing.T) {
	t.Parallel()

	b := NewBus[testEvent](4)
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	sub1 := b.Subscribe(ctx1)
	sub2 := b.Subscribe(ctx2)

	want := testEvent{Name: "payload", Value: 42}
	b.Publish(want)

	require.Equal(t, want, <-sub1)
	require.Equal(t, want, <-sub2)
}

func TestInMemoryBusClosesSubscriptionWhenContextIsDone(t *testing.T) {
	t.Parallel()

	b := NewBus[testEvent](1)
	ctx, cancel := context.WithCancel(context.Background())
	sub := b.Subscribe(ctx)

	cancel()

	select {
	case _, ok := <-sub:
		require.False(t, ok)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected subscription channel to close")
	}
}

func TestInMemoryBusShutdownClosesAllSubscribers(t *testing.T) {
	t.Parallel()

	b := NewBus[testEvent](1)
	ctx := context.Background()
	sub1 := b.Subscribe(ctx)
	sub2 := b.Subscribe(ctx)

	b.Shutdown()

	_, ok1 := <-sub1
	_, ok2 := <-sub2
	require.False(t, ok1)
	require.False(t, ok2)
}
