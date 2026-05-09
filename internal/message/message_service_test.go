package message

import (
	"context"
	"testing"
	"time"

	"github.com/YaHeii/agentGo/internal/store"
	"github.com/stretchr/testify/require"
)

func TestMessageServiceCreatePersistsMessageAndPublishesCreatedEvent(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := NewMessageService(st)

	msg, err := svc.Create(context.Background(), "session-1", CreateMessageParams{
		Kind:   KindUser,
		Status: StatusComplete,
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

	event := <-svc.Events()
	require.IsType(t, MessageCreatedEvent{}, event)
	require.Equal(t, msg.ID, event.(MessageCreatedEvent).Message.ID)
}

func TestMessageServiceCreatePersistsFinalRichMessageRecord(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := NewMessageService(st)

	msg, err := svc.Create(context.Background(), "session-1", CreateMessageParams{
		Kind:   KindUser,
		Status: StatusComplete,
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
}

func TestMessageServiceCreateUsesExplicitIDWhenProvided(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := NewMessageService(st)

	msg, err := svc.Create(context.Background(), "session-1", CreateMessageParams{
		ID:     "assistant-stream-1",
		Kind:   KindAssistant,
		Status: StatusComplete,
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

func TestMessageServiceListMapsStoredMessages(t *testing.T) {
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

	messages, err := svc.List(context.Background(), "session-1")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, KindUser, messages[0].Kind)
	require.Equal(t, "hello", messages[0].Parts[0].Text)
}

func TestMessageServiceUpdateStreamingPublishesDeltaEvent(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.createdMessages = append(st.createdMessages, store.Message{
		ID:               "assistant-1",
		SessionID:        "session-1",
		Kind:             string(KindAssistant),
		FinishedAt:       time.Unix(1710000000, 0).UTC(),
		IsCompactSummary: false,
		MessageJSON:      `{"flags":{},"parts":[{"Type":"text","Text":""}]}`,
	})
	st.messagesBySession["session-1"] = append(st.messagesBySession["session-1"], st.createdMessages[0])

	svc := NewMessageService(st)
	msg := Message{
		ID:        "assistant-1",
		SessionID: "session-1",
		Kind:      KindAssistant,
		Status:    StatusStreaming,
		Parts: []Part{
			{
				Type: PartTypeText,
				Text: "par",
			},
		},
	}

	err := svc.Update(context.Background(), msg)
	require.NoError(t, err)
	event := <-svc.Events()
	require.IsType(t, MessageDeltaEvent{}, event)
	require.Equal(t, "assistant-1", event.(MessageDeltaEvent).Message.ID)
	require.Equal(t, "par", event.(MessageDeltaEvent).Delta)
}

func TestMessageServiceUpdateCompletedPublishesCompletedEvent(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.createdMessages = append(st.createdMessages, store.Message{
		ID:               "assistant-1",
		SessionID:        "session-1",
		Kind:             string(KindAssistant),
		FinishedAt:       time.Unix(1710000000, 0).UTC(),
		IsCompactSummary: false,
		MessageJSON:      `{"flags":{},"parts":[{"Type":"text","Text":"par"}]}`,
	})
	st.messagesBySession["session-1"] = append(st.messagesBySession["session-1"], st.createdMessages[0])

	svc := NewMessageService(st)
	msg := Message{
		ID:        "assistant-1",
		SessionID: "session-1",
		Kind:      KindAssistant,
		Status:    StatusComplete,
		Parts: []Part{
			{
				Type: PartTypeText,
				Text: "done",
			},
		},
	}

	err := svc.Update(context.Background(), msg)
	require.NoError(t, err)

	event := <-svc.Events()
	require.IsType(t, MessageCompletedEvent{}, event)
	require.Equal(t, "assistant-1", event.(MessageCompletedEvent).Message.ID)
}

func TestMessageServiceUpdateFailedPublishesFailedEvent(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.createdMessages = append(st.createdMessages, store.Message{
		ID:               "assistant-1",
		SessionID:        "session-1",
		Kind:             string(KindAssistant),
		FinishedAt:       time.Unix(1710000000, 0).UTC(),
		IsCompactSummary: false,
		MessageJSON:      `{"flags":{},"parts":[{"Type":"text","Text":"par"}]}`,
	})
	st.messagesBySession["session-1"] = append(st.messagesBySession["session-1"], st.createdMessages[0])

	svc := NewMessageService(st)
	msg := Message{
		ID:        "assistant-1",
		SessionID: "session-1",
		Kind:      KindAssistant,
		Status:    StatusFailed,
		Parts: []Part{
			{
				Type: PartTypeText,
				Text: "par",
			},
		},
	}

	err := svc.Update(context.Background(), msg)
	require.NoError(t, err)

	event := <-svc.Events()
	require.IsType(t, MessageFailedEvent{}, event)
	require.Equal(t, "assistant-1", event.(MessageFailedEvent).Message.ID)
}

func TestMessageServiceUpdateCancelledPublishesCancelledEvent(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.createdMessages = append(st.createdMessages, store.Message{
		ID:               "assistant-1",
		SessionID:        "session-1",
		Kind:             string(KindAssistant),
		FinishedAt:       time.Unix(1710000000, 0).UTC(),
		IsCompactSummary: false,
		MessageJSON:      `{"flags":{},"parts":[{"Type":"text","Text":"par"}]}`,
	})
	st.messagesBySession["session-1"] = append(st.messagesBySession["session-1"], st.createdMessages[0])

	svc := NewMessageService(st)
	msg := Message{
		ID:        "assistant-1",
		SessionID: "session-1",
		Kind:      KindAssistant,
		Status:    StatusCancelled,
		Parts: []Part{
			{
				Type: PartTypeText,
				Text: "par",
			},
		},
	}

	err := svc.Update(context.Background(), msg)
	require.NoError(t, err)

	event := <-svc.Events()
	require.IsType(t, MessageCancelledEvent{}, event)
	require.Equal(t, "assistant-1", event.(MessageCancelledEvent).Message.ID)
}

type fakeStore struct {
	messagesBySession map[string][]store.Message
	createdMessages   []store.Message
	updatedMessages   []store.Message
	deletedDrafts     []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		messagesBySession: make(map[string][]store.Message),
	}
}

func (s *fakeStore) WithinTx(_ context.Context, fn func(tx store.TxStore) error) error {
	return fn(s)
}

func (s *fakeStore) CreateSession(_ context.Context, params store.CreateSessionParams) (store.Session, error) {
	return store.Session{
		ID:               params.ID,
		ParentSessionID:  params.ParentSessionID,
		Title:            params.Title,
		MessageCount:     params.MessageCount,
		CompletionTokens: params.CompletionTokens,
		CostMicros:       params.CostMicros,
		SummaryMessageID: params.SummaryMessageID,
		TodosJSON:        params.TodosJSON,
		CreatedAt:        params.CreatedAt,
		UpdatedAt:        params.UpdatedAt,
	}, nil
}

func (s *fakeStore) ListSessions(_ context.Context) ([]store.Session, error) {
	return nil, nil
}

func (s *fakeStore) GetSession(_ context.Context, _ string) (store.Session, error) {
	return store.Session{}, nil
}

func (s *fakeStore) UpdateSession(_ context.Context, _ store.UpdateSessionParams) (store.Session, error) {
	return store.Session{}, nil
}

func (s *fakeStore) DeleteSession(_ context.Context, _ string) error {
	return nil
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
	copied := append([]store.Message(nil), messages...)
	return copied, nil
}

func (s *fakeStore) LoadDraft(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (s *fakeStore) SaveDraft(_ context.Context, _ store.SaveDraftParams) error {
	return nil
}

func (s *fakeStore) DeleteDraft(_ context.Context, sessionID string) error {
	s.deletedDrafts = append(s.deletedDrafts, sessionID)
	return nil
}

func (s *fakeStore) Close() error {
	return nil
}
