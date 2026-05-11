package message

import (
	"context"
	"testing"
	"time"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/stretchr/testify/require"
)

func TestMessageServiceCreateMessagePersistsMessageAndDispatchesEvent(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	dispatcher := newStubDispatcher()
	svc := NewMessageService(st, dispatcher)

	msg, err := svc.CreateMessage(context.Background(), "session-1", CreateMessageParams{
		Kind: KindUser,
		Parts: []Part{
			{
				Type: PartTypeText,
				Text: "hello",
			},
		},
	}, dispatcher)
	require.NoError(t, err)
	require.Equal(t, "session-1", msg.SessionID)
	require.Equal(t, "hello", msg.Parts[0].Text)
	require.NotEmpty(t, msg.ID)

	require.Len(t, st.createdMessages, 1)
	require.Equal(t, "session-1", st.createdMessages[0].SessionID)
	require.Equal(t, string(KindUser), st.createdMessages[0].Kind)
	require.NotEmpty(t, st.createdMessages[0].MessageJSON)

	event := dispatcher.lastEvent
	require.NotNil(t, event)
	require.Equal(t, app.EventMessage, event.Type())

	payload, ok := event.Data().(MessageEvent)
	require.True(t, ok)
	require.Equal(t, StatusPending, payload.Status)
	require.NotNil(t, payload.Message)
	require.Equal(t, msg.ID, payload.Message.ID)
}

func TestMessageServiceCreateMessageUsesExplicitIDWhenProvided(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	dispatcher := newStubDispatcher()
	svc := NewMessageService(st, dispatcher)

	msg, err := svc.CreateMessage(context.Background(), "session-1", CreateMessageParams{
		ID:   "assistant-stream-1",
		Kind: KindAssistant,
		Parts: []Part{
			{
				Type: PartTypeText,
				Text: "hello",
			},
		},
	}, dispatcher)
	require.NoError(t, err)
	require.Equal(t, "assistant-stream-1", msg.ID)
	require.Len(t, st.createdMessages, 1)
	require.Equal(t, "assistant-stream-1", st.createdMessages[0].ID)
}

func TestMessageServiceCreateMessagePersistsFinalRichMessageRecord(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	dispatcher := newStubDispatcher()
	svc := NewMessageService(st, dispatcher)

	msg, err := svc.CreateMessage(context.Background(), "session-1", CreateMessageParams{
		Kind: KindUser,
		Parts: []Part{
			{
				Type: PartTypeText,
				Text: "hello",
			},
		},
	}, dispatcher)
	require.NoError(t, err)
	require.Equal(t, KindUser, msg.Kind)
	require.NotEmpty(t, st.createdMessages)
	require.NotEmpty(t, st.createdMessages[0].MessageJSON)
	require.False(t, st.createdMessages[0].FinishedAt.IsZero())
}

func TestMessageServiceListMessagesMapsStoredMessages(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	dispatcher := newStubDispatcher()
	st.messagesBySession["session-1"] = []store.Message{
		{
			ID:               "u1",
			SessionID:        "session-1",
			Kind:             string(KindUser),
			Provider:         "",
			FinishedAt:       time.Unix(1710000000, 0).UTC(),
			IsCompactSummary: false,
			MessageJSON:      `{"flags":{},"parts":[{"Type":"text","Text":"hello"}]}`,
		},
	}

	svc := NewMessageService(st, dispatcher)

	messages, err := svc.ListMessages(context.Background(), "session-1", dispatcher)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, KindUser, messages[0].Kind)
	require.Equal(t, "hello", messages[0].Parts[0].Text)
}

type fakeStore struct {
	messagesBySession map[string][]store.Message
	createdMessages   []store.Message
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		messagesBySession: make(map[string][]store.Message),
	}
}

func (s *fakeStore) CreateMessage(_ context.Context, params store.CreateMessageParams) (store.Message, error) {
	msg := store.Message{
		ID:               params.ID,
		SessionID:        params.SessionID,
		Kind:             params.Kind,
		Provider:         params.Provider,
		FinishedAt:       params.FinishedAt,
		IsCompactSummary: params.IsCompactSummary,
		MessageJSON:      params.MessageJSON,
	}
	s.createdMessages = append(s.createdMessages, msg)
	s.messagesBySession[params.SessionID] = append(s.messagesBySession[params.SessionID], msg)
	return msg, nil
}

func (s *fakeStore) ListMessages(_ context.Context, sessionID string) ([]store.Message, error) {
	messages := s.messagesBySession[sessionID]
	return append([]store.Message(nil), messages...), nil
}

type stubDispatcher struct {
	lastEvent app.Event
}

func newStubDispatcher() *stubDispatcher {
	return &stubDispatcher{}
}

func (d *stubDispatcher) Dispatch(evt app.Event) {
	d.lastEvent = evt
}

func (d *stubDispatcher) Subscribe(context.Context) <-chan app.Event {
	return make(chan app.Event)
}
