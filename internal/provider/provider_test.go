package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/stretchr/testify/require"
)

func TestStreamingLLMContractUsesRequest(t *testing.T) {
	t.Parallel()

	var llm StreamingLLM = stubStreamingLLM{}
	req := Request{
		SessionID: "session-1",
	}

	stream := llm.StreamChat(context.Background(), req)
	if stream == nil {
		t.Fatal("expected request-based stream channel")
	}
}

func TestRequestCarriesSessionID(t *testing.T) {
	t.Parallel()

	req := Request{
		SessionID: "session-1",
	}

	if req.SessionID != "session-1" {
		t.Fatalf("unexpected session id: %q", req.SessionID)
	}
}

func TestProviderServiceStreamsMessagesFromStoreAndPublishesEvents(t *testing.T) {
	t.Parallel()

	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())
	store := &stubMessageStore{
		messages: []message.Message{
			{
				ID:   "message-1",
				Kind: message.KindUser,
				Parts: []message.Part{
					{
						Type: message.PartTypeText,
						Text: "hello",
					},
				},
			},
		},
	}
	client := &stubStreamClient{
		events: []StreamEvent{
			{Type: StreamEventTextDelta, TextDelta: "hi"},
			{Type: StreamEventTurnFinished, StopReason: StopReasonStop},
		},
	}

	svc := NewProviderService(store, client, dispatcher)
	stream := svc.StreamChat(context.Background(), Request{SessionID: "session-1"})

	got := collectStreamEvents(stream)

	require.Equal(t, []string{"session-1"}, store.sessionIDs)
	require.Len(t, client.calls, 1)
	require.Equal(t, store.messages, client.calls[0])
	require.Equal(t, client.events, got)

	firstEvent := <-events
	require.Equal(t, app.EventProvider, firstEvent.Type())
	require.Equal(t, StreamEventTextDelta, firstEvent.Data().(StreamEvent).Type)

	secondEvent := <-events
	require.Equal(t, app.EventProvider, secondEvent.Type())
	require.Equal(t, StreamEventTurnFinished, secondEvent.Data().(StreamEvent).Type)
}

func TestProviderServiceEmitsProviderErrorWhenStoreFails(t *testing.T) {
	t.Parallel()

	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())
	store := &stubMessageStore{err: errors.New("list failed")}
	client := &stubStreamClient{}

	svc := NewProviderService(store, client, dispatcher)
	got := collectStreamEvents(svc.StreamChat(context.Background(), Request{SessionID: "session-1"}))

	require.Len(t, got, 1)
	require.Equal(t, StreamEventProviderError, got[0].Type)
	require.EqualError(t, got[0].Err, "list provider messages: list failed")
	require.Len(t, client.calls, 0)

	published := <-events
	require.Equal(t, app.EventProvider, published.Type())
	require.Equal(t, got[0], published.Data().(StreamEvent))
}

func TestStreamEventTypesAreStable(t *testing.T) {
	t.Parallel()

	cases := map[StreamEventType]string{
		StreamEventTextDelta:         "text_delta",
		StreamEventReasoningDelta:    "reasoning_delta",
		StreamEventRefusalDelta:      "refusal_delta",
		StreamEventToolCallDelta:     "tool_call_delta",
		StreamEventToolCallCompleted: "tool_call_completed",
		StreamEventUsageAvailable:    "usage_available",
		StreamEventTurnFinished:      "turn_finished",
		StreamEventProviderError:     "provider_error",
	}

	for got, want := range cases {
		if string(got) != want {
			t.Fatalf("unexpected stream event type: got %q want %q", got, want)
		}
	}
}

type stubStreamingLLM struct{}

func (stubStreamingLLM) StreamChat(_ context.Context, _ Request) <-chan StreamEvent {
	return make(chan StreamEvent)
}

type stubMessageStore struct {
	messages   []message.Message
	sessionIDs []string
	err        error
}

func (s *stubMessageStore) ListMessages(_ context.Context, sessionID string, _ app.Dispatcher) ([]message.Message, error) {
	s.sessionIDs = append(s.sessionIDs, sessionID)
	if s.err != nil {
		return nil, s.err
	}
	return s.messages, nil
}

type stubStreamClient struct {
	events []StreamEvent
	calls  [][]message.Message
}

func (s *stubStreamClient) streamMessages(_ context.Context, messages []message.Message) <-chan StreamEvent {
	s.calls = append(s.calls, messages)
	ch := make(chan StreamEvent, len(s.events))
	for _, event := range s.events {
		ch <- event
	}
	close(ch)
	return ch
}

func collectStreamEvents(ch <-chan StreamEvent) []StreamEvent {
	events := make([]StreamEvent, 0)
	for event := range ch {
		events = append(events, event)
	}
	return events
}
