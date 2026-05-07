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
		Origin: OriginHuman,
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

func TestMessageServiceListMapsStoredMessages(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.messagesBySession["session-1"] = []store.Message{
		{
			ID:        "u1",
			SessionID: "session-1",
			Role:      "user",
			Content:   "hello",
			Status:    store.MessageStatusComplete,
			CreatedAt: time.Unix(1710000000, 0).UTC(),
			UpdatedAt: time.Unix(1710000000, 0).UTC(),
		},
	}

	svc := NewMessageService(st)

	messages, err := svc.List(context.Background(), "session-1")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, KindUser, messages[0].Kind)
	require.Equal(t, "hello", messages[0].Parts[0].Text)
}

func TestMessageServiceUpdatePersistsMessageAndPublishesCompletedEvent(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.createdMessages = append(st.createdMessages, store.Message{
		ID:        "assistant-1",
		SessionID: "session-1",
		Role:      "assistant",
		Content:   "",
		Status:    store.MessageStatusStreaming,
		CreatedAt: time.Unix(1710000000, 0).UTC(),
		UpdatedAt: time.Unix(1710000000, 0).UTC(),
	})
	st.messagesBySession["session-1"] = append(st.messagesBySession["session-1"], st.createdMessages[0])

	svc := NewMessageService(st)
	msg := Message{
		ID:        "assistant-1",
		SessionID: "session-1",
		Kind:      KindAssistant,
		Origin:    OriginModel,
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
	require.Len(t, st.updatedMessages, 1)
	require.Equal(t, "done", st.updatedMessages[0].Content)
	require.Equal(t, store.MessageStatusComplete, st.updatedMessages[0].Status)

	event := <-svc.Events()
	require.IsType(t, MessageCompletedEvent{}, event)
	require.Equal(t, "assistant-1", event.(MessageCompletedEvent).Message.ID)
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
		ID:           params.ID,
		Title:        params.Title,
		CreatedAt:    params.CreatedAt,
		UpdatedAt:    params.UpdatedAt,
		LastActiveAt: params.LastActiveAt,
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
		ID:        params.ID,
		SessionID: params.SessionID,
		Role:      params.Role,
		Content:   params.Content,
		Status:    params.Status,
		CreatedAt: params.CreatedAt,
		UpdatedAt: params.UpdatedAt,
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

func (s *fakeStore) UpdateMessage(_ context.Context, params store.UpdateMessageParams) (store.Message, error) {
	for i := range s.createdMessages {
		if s.createdMessages[i].ID != params.ID {
			continue
		}
		s.createdMessages[i].Content = params.Content
		s.createdMessages[i].Status = params.Status
		s.createdMessages[i].UpdatedAt = params.UpdatedAt
		s.updatedMessages = append(s.updatedMessages, s.createdMessages[i])

		for j := range s.messagesBySession[s.createdMessages[i].SessionID] {
			if s.messagesBySession[s.createdMessages[i].SessionID][j].ID == params.ID {
				s.messagesBySession[s.createdMessages[i].SessionID][j] = s.createdMessages[i]
			}
		}

		return s.createdMessages[i], nil
	}
	return store.Message{}, store.ErrMessageNotFound
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
