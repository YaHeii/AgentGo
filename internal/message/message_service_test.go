package message

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/stretchr/testify/require"
)

func TestMessageServicePublishesCreatedDeltaAndCompletedEvents(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	llm := &fakeStreamingLLM{
		events: []provider.StreamEvent{
			{Type: provider.StreamEventDelta, Delta: "hel"},
			{Type: provider.StreamEventDelta, Delta: "lo"},
			{Type: provider.StreamEventDone},
		},
	}

	svc := NewMessageService(st, llm, timeNowStub)

	result, err := svc.SendMessage(context.Background(), SendMessageParams{
		SessionID: "session-1",
		Prompt:    "hi",
	})
	require.NoError(t, err)
	require.Equal(t, "hello", result.Assistant.Content)

	events := []Event{
		<-svc.Events(),
		<-svc.Events(),
		<-svc.Events(),
		<-svc.Events(),
		<-svc.Events(),
	}

	require.IsType(t, MessageCreatedEvent{}, events[0])
	require.IsType(t, MessageCreatedEvent{}, events[1])
	require.IsType(t, MessageDeltaEvent{}, events[2])
	require.IsType(t, MessageDeltaEvent{}, events[3])
	require.IsType(t, MessageCompletedEvent{}, events[4])
	require.Equal(t, "hel", events[2].(MessageDeltaEvent).Delta)
	require.Equal(t, "hello", events[4].(MessageCompletedEvent).Message.Content)
	require.Equal(t, StatusComplete, events[4].(MessageCompletedEvent).Message.Status)
}

func TestMessageServicePublishesFailedEventWhenStreamErrors(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	llm := &fakeStreamingLLM{
		events: []provider.StreamEvent{
			{Type: provider.StreamEventDelta, Delta: "par"},
			{Err: errors.New("stream failed")},
		},
	}

	svc := NewMessageService(st, llm, timeNowStub)

	result, err := svc.SendMessage(context.Background(), SendMessageParams{
		SessionID: "session-1",
		Prompt:    "hi",
	})
	require.Error(t, err)
	require.Equal(t, "par", result.Assistant.Content)
	require.Equal(t, StatusFailed, result.Assistant.Status)

	events := []Event{
		<-svc.Events(),
		<-svc.Events(),
		<-svc.Events(),
		<-svc.Events(),
	}

	require.IsType(t, MessageCreatedEvent{}, events[0])
	require.IsType(t, MessageCreatedEvent{}, events[1])
	require.IsType(t, MessageDeltaEvent{}, events[2])
	require.IsType(t, MessageFailedEvent{}, events[3])
	require.EqualError(t, events[3].(MessageFailedEvent).Err, "stream failed")
}

func TestMessageServiceBuildsProviderHistoryWithoutAssistantPlaceholder(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.messagesBySession["session-1"] = []store.Message{
		{ID: "u1", SessionID: "session-1", Role: "user", Content: "first"},
		{ID: "a1", SessionID: "session-1", Role: "assistant", Content: "second"},
	}

	llm := &fakeStreamingLLM{
		events: []provider.StreamEvent{
			{Type: provider.StreamEventDone},
		},
	}

	svc := NewMessageService(st, llm, timeNowStub)

	_, err := svc.SendMessage(context.Background(), SendMessageParams{
		SessionID: "session-1",
		Prompt:    "third",
	})
	require.NoError(t, err)
	require.Len(t, llm.calls, 1)
	require.Len(t, llm.calls[0], 3)
	require.Equal(t, "first", llm.calls[0][0].Content)
	require.Equal(t, "second", llm.calls[0][1].Content)
	require.Equal(t, "third", llm.calls[0][2].Content)
}

func timeNowStub() time.Time {
	return time.Unix(1710004000, 0).UTC()
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

type fakeStreamingLLM struct {
	events []provider.StreamEvent
	calls  [][]provider.Message
}

func (f *fakeStreamingLLM) StreamChat(_ context.Context, messages []provider.Message) <-chan provider.StreamEvent {
	copied := append([]provider.Message(nil), messages...)
	f.calls = append(f.calls, copied)

	ch := make(chan provider.StreamEvent, len(f.events))
	for _, event := range f.events {
		ch <- event
	}
	close(ch)
	return ch
}
