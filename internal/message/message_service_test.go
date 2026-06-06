package message

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
)

func TestMessageServiceCreateMessagePersistsMessage(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := NewMessageService(st)

	msg, err := svc.CreateMessage(context.Background(), messagecontract.CreateMessageParams{
		SessionID: "session-1",
		Kind:      messagecontract.KindUser,
		Parts: []messagecontract.Part{
			{
				Type: messagecontract.PartTypeText,
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
	require.Equal(t, string(messagecontract.KindUser), st.createdMessages[0].Kind)
	require.NotEmpty(t, st.createdMessages[0].MessageJSON)
}

func TestMessageServiceCreateMessageGeneratesKSUIDWhenIDOmitted(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := NewMessageService(st)

	msg, err := svc.CreateMessage(context.Background(), messagecontract.CreateMessageParams{
		SessionID: "session-1",
		Kind:      messagecontract.KindAssistant,
		Parts: []messagecontract.Part{
			{
				Type: messagecontract.PartTypeText,
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

	msg, err := svc.CreateMessage(context.Background(), messagecontract.CreateMessageParams{
		ID:        "assistant-stream-1",
		SessionID: "session-1",
		Kind:      messagecontract.KindAssistant,
		Parts: []messagecontract.Part{
			{
				Type: messagecontract.PartTypeText,
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

	msg, err := svc.CreateMessage(context.Background(), messagecontract.CreateMessageParams{
		SessionID: "session-1",
		Kind:      messagecontract.KindUser,
		Parts: []messagecontract.Part{
			{
				Type: messagecontract.PartTypeText,
				Text: "hello",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, messagecontract.KindUser, msg.Kind)
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

	msg, err := svc.CreateMessage(context.Background(), messagecontract.CreateMessageParams{
		SessionID:        "session-1",
		Kind:             messagecontract.KindAssistant,
		IsCompactSummary: true,
		Parts: []messagecontract.Part{
			{
				Type: messagecontract.PartTypeText,
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
	st.messagesBySession["session-1"] = []MessageRecord{
		{
			ID:               "u1",
			SessionID:        "session-1",
			Kind:             string(messagecontract.KindUser),
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
	require.Equal(t, messagecontract.KindUser, messages[0].Kind)
	require.Equal(t, "hello", messages[0].Parts[0].Text)
	require.False(t, messages[0].IsCompactSummary)
}

func TestMessageServiceCreateMessagePreservesSystemAndProgressPayloads(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := NewMessageService(st)

	msg, err := svc.CreateMessage(context.Background(), messagecontract.CreateMessageParams{
		SessionID: "session-1",
		Kind:      messagecontract.KindSystem,
		Parts: []messagecontract.Part{
			{
				Type: messagecontract.PartTypeText,
				Text: "working",
			},
		},
		System: &messagecontract.SystemPayload{
			Subtype: "informational",
			Level:   "info",
		},
		Progress: &messagecontract.ProgressPayload{
			Stage:   "thinking",
			Current: 1,
			Total:   2,
			Done:    false,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, msg.System)
	require.Equal(t, "informational", msg.System.Subtype)
	require.NotNil(t, msg.Progress)
	require.Equal(t, "thinking", msg.Progress.Stage)

	require.Len(t, st.createdMessages, 1)

	var payload struct {
		System   *messagecontract.SystemPayload   `json:"system"`
		Progress *messagecontract.ProgressPayload `json:"progress"`
	}
	require.NoError(t, json.Unmarshal([]byte(st.createdMessages[0].MessageJSON), &payload))
	require.NotNil(t, payload.System)
	require.Equal(t, "informational", payload.System.Subtype)
	require.NotNil(t, payload.Progress)
	require.Equal(t, "thinking", payload.Progress.Stage)
}

func TestMessageServiceGetReturnsStoredMessage(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.messagesBySession["session-1"] = []MessageRecord{
		{
			ID:               "m1",
			SessionID:        "session-1",
			Kind:             string(messagecontract.KindAssistant),
			FinishedAt:       time.Unix(1710000000, 0).UTC(),
			IsCompactSummary: false,
			MessageJSON:      `{"parts":[{"Type":"text","Text":"hello"}]}`,
		},
	}

	svc := NewMessageService(st)

	msg, err := svc.Get(context.Background(), "m1")
	require.NoError(t, err)
	require.Equal(t, "m1", msg.ID)
	require.Equal(t, "hello", msg.Parts[0].Text)
}

func TestMessageServiceDeleteRemovesMessageByID(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.messagesBySession["session-1"] = []MessageRecord{
		{
			ID:          "m1",
			SessionID:   "session-1",
			Kind:        string(messagecontract.KindUser),
			FinishedAt:  time.Unix(1710000000, 0).UTC(),
			MessageJSON: `{"parts":[{"Type":"text","Text":"hello"}]}`,
		},
		{
			ID:          "m2",
			SessionID:   "session-1",
			Kind:        string(messagecontract.KindAssistant),
			FinishedAt:  time.Unix(1710000001, 0).UTC(),
			MessageJSON: `{"parts":[{"Type":"text","Text":"world"}]}`,
		},
	}

	svc := NewMessageService(st)

	err := svc.Delete(context.Background(), "m1")
	require.NoError(t, err)
	require.Equal(t, []string{"m1"}, st.deletedMessageIDs)
	require.Len(t, st.messagesBySession["session-1"], 1)
	require.Equal(t, "m2", st.messagesBySession["session-1"][0].ID)
}

func TestMessageServiceDeleteSessionMessagesRemovesAllSessionMessages(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.messagesBySession["session-1"] = []MessageRecord{
		{
			ID:          "m1",
			SessionID:   "session-1",
			Kind:        string(messagecontract.KindUser),
			FinishedAt:  time.Unix(1710000000, 0).UTC(),
			MessageJSON: `{"parts":[{"Type":"text","Text":"hello"}]}`,
		},
	}
	st.messagesBySession["session-2"] = []MessageRecord{
		{
			ID:          "m2",
			SessionID:   "session-2",
			Kind:        string(messagecontract.KindAssistant),
			FinishedAt:  time.Unix(1710000001, 0).UTC(),
			MessageJSON: `{"parts":[{"Type":"text","Text":"world"}]}`,
		},
	}

	svc := NewMessageService(st)

	err := svc.DeleteSessionMessages(context.Background(), "session-1")
	require.NoError(t, err)
	require.Equal(t, []string{"session-1"}, st.deletedSessionIDs)
	require.Empty(t, st.messagesBySession["session-1"])
	require.Len(t, st.messagesBySession["session-2"], 1)
}

type fakeStore struct {
	messagesBySession map[string][]MessageRecord
	createdMessages   []MessageRecord
	deletedMessageIDs []string
	deletedSessionIDs []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		messagesBySession: make(map[string][]MessageRecord),
	}
}

func (s *fakeStore) CreateMessage(_ context.Context, params CreateMessageRecordParams) (MessageRecord, error) {
	msg := MessageRecord{
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

func (s *fakeStore) ListMessages(_ context.Context, sessionID string) ([]MessageRecord, error) {
	messages := s.messagesBySession[sessionID]
	return append([]MessageRecord(nil), messages...), nil
}

func (s *fakeStore) GetMessage(_ context.Context, id string) (MessageRecord, error) {
	for _, messages := range s.messagesBySession {
		for _, msg := range messages {
			if msg.ID == id {
				return msg, nil
			}
		}
	}
	return MessageRecord{}, errTODO
}

func (s *fakeStore) DeleteMessage(_ context.Context, id string) error {
	s.deletedMessageIDs = append(s.deletedMessageIDs, id)
	for sessionID, messages := range s.messagesBySession {
		filtered := messages[:0]
		for _, msg := range messages {
			if msg.ID != id {
				filtered = append(filtered, msg)
			}
		}
		s.messagesBySession[sessionID] = append([]MessageRecord(nil), filtered...)
	}
	return nil
}

func (s *fakeStore) DeleteSessionMessages(_ context.Context, sessionID string) error {
	s.deletedSessionIDs = append(s.deletedSessionIDs, sessionID)
	delete(s.messagesBySession, sessionID)
	return nil
}
