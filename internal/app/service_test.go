package app

import (
	"context"
	"testing"
	"time"

	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/stretchr/testify/require"
)

func TestServiceEnsureActiveSessionCreatesFirstSessionAndPublishesBootstrapEvents(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	llm := &fakeStreamingLLM{}
	svc := NewService(st, llm, timeNowStub)

	session, err := svc.EnsureActiveSession(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, session.ID)
	require.Len(t, st.sessions, 1)

	first := <-svc.Events()
	second := <-svc.Events()

	require.IsType(t, SessionReadyEvent{}, first)
	require.IsType(t, ConversationHydratedEvent{}, second)
	require.Equal(t, session.ID, first.(SessionReadyEvent).Session.ID)
	require.Equal(t, session.ID, second.(ConversationHydratedEvent).SessionID)
	require.Len(t, second.(ConversationHydratedEvent).Messages, 0)
}

func TestServiceEnsureActiveSessionLoadsMostRecentSessionHistory(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.sessions = []store.Session{
		{
			ID:           "session-2",
			Title:        "latest",
			CreatedAt:    time.Unix(1710000200, 0).UTC(),
			UpdatedAt:    time.Unix(1710000200, 0).UTC(),
			LastActiveAt: time.Unix(1710000200, 0).UTC(),
		},
		{
			ID:           "session-1",
			Title:        "older",
			CreatedAt:    time.Unix(1710000000, 0).UTC(),
			UpdatedAt:    time.Unix(1710000000, 0).UTC(),
			LastActiveAt: time.Unix(1710000000, 0).UTC(),
		},
	}
	st.messagesBySession["session-2"] = []store.Message{
		{ID: "u1", SessionID: "session-2", Role: "user", Content: "hello"},
		{ID: "a1", SessionID: "session-2", Role: "assistant", Content: "world"},
	}

	svc := NewService(st, &fakeStreamingLLM{}, timeNowStub)

	session, err := svc.EnsureActiveSession(context.Background())
	require.NoError(t, err)
	require.Equal(t, "session-2", session.ID)

	<-svc.Events()
	hydrated := (<-svc.Events()).(ConversationHydratedEvent)
	require.Equal(t, "session-2", hydrated.SessionID)
	require.Len(t, hydrated.Messages, 2)
	require.Equal(t, "hello", hydrated.Messages[0].Content)
	require.Equal(t, "world", hydrated.Messages[1].Content)
}

func TestServiceSendMessageDelegatesToMessageService(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.sessions = []store.Session{
		{ID: "session-1", Title: "demo"},
	}

	llm := &fakeStreamingLLM{
		events: []provider.StreamEvent{
			{Type: provider.StreamEventDone},
		},
	}

	svc := NewService(st, llm, timeNowStub)

	_, err := svc.SendMessage(context.Background(), SendMessageParams{
		SessionID: "session-1",
		Prompt:    "hello",
	})
	require.NoError(t, err)
	require.Len(t, llm.calls, 1)
}

func TestServiceUsesMessageServiceInterface(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := &Service{
		store:   st,
		bus:     nil,
		nowFunc: timeNowStub,
		query:   stubQueryRunner{},
	}

	_, err := svc.SendMessage(context.Background(), SendMessageParams{
		SessionID: "session-1",
		Prompt:    "hello",
	})
	require.NoError(t, err)
}

func timeNowStub() time.Time {
	return time.Unix(1710004000, 0).UTC()
}

type fakeStore struct {
	sessions          []store.Session
	messagesBySession map[string][]store.Message
	createdMessages   []store.Message
	updatedMessages   []store.Message
	updatedSessions   []store.UpdateSessionParams
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
	session := store.Session{
		ID:           params.ID,
		Title:        params.Title,
		CreatedAt:    params.CreatedAt,
		UpdatedAt:    params.UpdatedAt,
		LastActiveAt: params.LastActiveAt,
	}
	s.sessions = append([]store.Session{session}, s.sessions...)
	return session, nil
}

func (s *fakeStore) ListSessions(_ context.Context) ([]store.Session, error) {
	copied := append([]store.Session(nil), s.sessions...)
	return copied, nil
}

func (s *fakeStore) GetSession(_ context.Context, id string) (store.Session, error) {
	for _, session := range s.sessions {
		if session.ID == id {
			return session, nil
		}
	}
	return store.Session{}, store.ErrSessionNotFound
}

func (s *fakeStore) UpdateSession(_ context.Context, params store.UpdateSessionParams) (store.Session, error) {
	s.updatedSessions = append(s.updatedSessions, params)
	for i := range s.sessions {
		if s.sessions[i].ID != params.ID {
			continue
		}
		if params.Title != "" {
			s.sessions[i].Title = params.Title
		}
		s.sessions[i].UpdatedAt = params.UpdatedAt
		s.sessions[i].LastActiveAt = params.LastActiveAt
		return s.sessions[i], nil
	}
	return store.Session{}, store.ErrSessionNotFound
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

type stubQueryRunner struct{}

func (stubQueryRunner) RunQuery(_ context.Context, _ agent.QueryParams) (agent.QueryResult, error) {
	return agent.QueryResult{}, nil
}
