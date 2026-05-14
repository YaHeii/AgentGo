package message

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/YaHeii/agentGo/internal/store"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
)

func TestMessageServiceCreateMessagePersistsMessage(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := NewMessageService(st)

	msg, err := svc.CreateMessage(context.Background(), CreateMessageParams{
		SessionID: "session-1",
		Kind:      KindUser,
		Parts: []Part{
			{
				Type: PartTypeText,
				Text: "hello",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "session-1", msg.SessionID)
	require.Equal(t, "hello", msg.Parts[0].Text)
	require.NotEmpty(t, msg.ID)

	require.Len(t, st.createdMessages, 1)
	require.Equal(t, "session-1", st.createdMessages[0].SessionID)
	require.Equal(t, string(KindUser), st.createdMessages[0].Kind)
	require.NotEmpty(t, st.createdMessages[0].MessageJSON)
}

func TestMessageServiceCreateMessageGeneratesKSUIDWhenIDOmitted(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := NewMessageService(st)

	msg, err := svc.CreateMessage(context.Background(), CreateMessageParams{
		SessionID: "session-1",
		Kind:      KindAssistant,
		Parts: []Part{
			{
				Type: PartTypeText,
				Text: "hello",
			},
		},
	})
	require.NoError(t, err)
	_, parseErr := ksuid.Parse(msg.ID)
	require.NoError(t, parseErr)
	require.Len(t, st.createdMessages, 1)
	require.Equal(t, msg.ID, st.createdMessages[0].ID)
}

func TestMessageServiceCreateMessageUsesExplicitIDWhenProvided(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := NewMessageService(st)

	msg, err := svc.CreateMessage(context.Background(), CreateMessageParams{
		ID:        "assistant-stream-1",
		SessionID: "session-1",
		Kind:      KindAssistant,
		Parts: []Part{
			{
				Type: PartTypeText,
				Text: "hello",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "assistant-stream-1", msg.ID)
	require.Len(t, st.createdMessages, 1)
	require.Equal(t, "assistant-stream-1", st.createdMessages[0].ID)
}

func TestMessageServiceCreateMessagePersistsFinalRichMessageRecord(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := NewMessageService(st)

	msg, err := svc.CreateMessage(context.Background(), CreateMessageParams{
		SessionID: "session-1",
		Kind:      KindUser,
		Parts: []Part{
			{
				Type: PartTypeText,
				Text: "hello",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, KindUser, msg.Kind)
	require.NotEmpty(t, st.createdMessages)
	require.NotEmpty(t, st.createdMessages[0].MessageJSON)
	require.False(t, st.createdMessages[0].FinishedAt.IsZero())

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(st.createdMessages[0].MessageJSON), &payload))
	require.Contains(t, payload, "parts")
	require.NotContains(t, payload, "flags")
}

func TestMessageServiceCreateMessagePersistsCompactSummaryFlagInStoreRecord(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := NewMessageService(st)

	msg, err := svc.CreateMessage(context.Background(), CreateMessageParams{
		SessionID:        "session-1",
		Kind:             KindAssistant,
		IsCompactSummary: true,
		Parts: []Part{
			{
				Type: PartTypeText,
				Text: "summary",
			},
		},
	})
	require.NoError(t, err)
	require.True(t, msg.IsCompactSummary)
	require.Len(t, st.createdMessages, 1)
	require.True(t, st.createdMessages[0].IsCompactSummary)
}

func TestMessageServiceListMessagesMapsStoredMessages(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
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

	svc := NewMessageService(st)

	messages, err := svc.ListMessages(context.Background(), "session-1")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, KindUser, messages[0].Kind)
	require.Equal(t, "hello", messages[0].Parts[0].Text)
	require.False(t, messages[0].IsCompactSummary)
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
