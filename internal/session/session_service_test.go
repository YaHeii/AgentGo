package session

import (
	"context"
	"testing"
	"time"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/stretchr/testify/require"
)

func TestSessionServiceCreatePublishesCreatedEvent(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	dispatcher := newStubDispatcher()
	svc := NewSessionService(st, newFakeSessionMessages(), dispatcher)
	svc.nowFunc = timeNowStub

	sessionID, err := svc.Create(context.Background(), "New Session", dispatcher)
	require.NoError(t, err)
	require.NotEmpty(t, sessionID)
	require.Len(t, st.sessions, 1)
	require.Equal(t, sessionID, st.sessions[0].ID)
	require.Equal(t, "New Session", st.sessions[0].Title)

	event := dispatcher.lastEvent
	require.NotNil(t, event)
	require.Equal(t, app.EventSession, event.Type())

	payload, ok := event.Data().(SessionEvent)
	require.True(t, ok)
	require.Equal(t, StatusCreated, payload.Status)
	require.NotNil(t, payload.Session)
	require.Equal(t, sessionID, payload.Session.ID)
}

func TestSessionServiceGetLastReturnsNotFoundWhenEmpty(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	svc := NewSessionService(st, newFakeSessionMessages(), newStubDispatcher())
	svc.nowFunc = timeNowStub

	_, err := svc.GetLast(context.Background())
	require.ErrorIs(t, err, ErrSessionNotFound)
}

func TestSessionServiceRenameUpdatesSessionAndPublishesUpdatedEvent(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	st.sessions = []store.Session{
		{
			ID:        "session-1",
			Title:     "old",
			TodosJSON: "[]",
			CreatedAt: time.Unix(1710000000, 0).UTC(),
			UpdatedAt: time.Unix(1710000000, 0).UTC(),
		},
	}
	dispatcher := newStubDispatcher()
	svc := NewSessionService(st, newFakeSessionMessages(), dispatcher)
	svc.nowFunc = timeNowStub

	session, err := svc.Rename(context.Background(), "session-1", "new", dispatcher)
	require.NoError(t, err)
	require.Equal(t, "new", session.Title)

	event := dispatcher.lastEvent
	require.NotNil(t, event)

	payload, ok := event.Data().(SessionEvent)
	require.True(t, ok)
	require.Equal(t, StatusUpdated, payload.Status)
	require.NotNil(t, payload.Session)
	require.Equal(t, "new", payload.Session.Title)
}

func TestSessionServiceGetLastUsesUpdatedAtOrdering(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	st.sessions = []store.Session{
		{
			ID:        "session-2",
			Title:     "latest",
			CreatedAt: time.Unix(1710000000, 0).UTC(),
			UpdatedAt: time.Unix(1710000500, 0).UTC(),
		},
		{
			ID:        "session-1",
			Title:     "older",
			CreatedAt: time.Unix(1710000000, 0).UTC(),
			UpdatedAt: time.Unix(1710000100, 0).UTC(),
		},
	}
	svc := NewSessionService(st, newFakeSessionMessages(), newStubDispatcher())
	svc.nowFunc = timeNowStub

	got, err := svc.GetLast(context.Background())
	require.NoError(t, err)
	require.Equal(t, "session-2", got)
}

func TestSessionServiceListHistoryDelegatesToMessageDependency(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	msgs := newFakeSessionMessages()
	dispatcher := newStubDispatcher()
	msgs.listResult["session-1"] = []message.Message{
		{ID: "message-1", SessionID: "session-1", Kind: message.KindUser},
	}
	svc := NewSessionService(st, msgs, dispatcher)
	svc.nowFunc = timeNowStub

	got, err := svc.ListHistory(context.Background(), "session-1", dispatcher)
	require.NoError(t, err)
	require.Equal(t, []message.Message{{ID: "message-1", SessionID: "session-1", Kind: message.KindUser}}, got)
	require.Equal(t, "session-1", msgs.lastListSessionID)
}

func TestSessionServiceCreateMessageRoutesThroughMessageDependency(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	msgs := newFakeSessionMessages()
	dispatcher := newStubDispatcher()
	msgs.createResult = message.Message{ID: "message-1", SessionID: "session-1", Kind: message.KindAssistant}
	svc := NewSessionService(st, msgs, dispatcher)
	svc.nowFunc = timeNowStub

	got, err := svc.CreateMessage(context.Background(), "session-1", message.CreateMessageParams{Kind: message.KindAssistant}, dispatcher)
	require.NoError(t, err)
	require.Equal(t, msgs.createResult, got)
	require.Equal(t, "session-1", msgs.lastCreateSessionID)
	require.Equal(t, message.KindAssistant, msgs.lastCreateParams.Kind)
}

func TestSessionServiceSwitchSessionUpdatesStateAndPublishesEvent(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	st.sessions = []store.Session{
		{
			ID:              "session-1",
			ParentSessionID: "parent-1",
		},
	}
	dispatcher := newStubDispatcher()
	svc := NewSessionService(st, newFakeSessionMessages(), dispatcher)
	svc.nowFunc = timeNowStub

	err := svc.SwitchSession(context.Background(), "session-1", dispatcher)
	require.NoError(t, err)
	require.Equal(t, "session-1", svc.GetSessionID())
	require.Equal(t, "parent-1", svc.GetParentSessionID())

	event := dispatcher.lastEvent
	require.NotNil(t, event)

	payload, ok := event.Data().(SessionEvent)
	require.True(t, ok)
	require.Equal(t, StatusUpdated, payload.Status)
	require.NotNil(t, payload.Session)
	require.Equal(t, "session-1", payload.Session.ID)
}

func TestSessionServiceSwitchSessionReturnsNotFoundWhenMissing(t *testing.T) {
	t.Parallel()

	dispatcher := newStubDispatcher()
	svc := NewSessionService(newFakeSessionStore(), newFakeSessionMessages(), dispatcher)
	svc.nowFunc = timeNowStub

	err := svc.SwitchSession(context.Background(), "missing", dispatcher)
	require.ErrorIs(t, err, ErrSessionNotFound)
}

func TestSessionServiceRestoreReturnsOnlyErrorAndPublishesEvent(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	st.sessions = []store.Session{
		{
			ID:        "session-1",
			Title:     "restored",
			TodosJSON: "[]",
			CreatedAt: time.Unix(1710000000, 0).UTC(),
			UpdatedAt: time.Unix(1710000100, 0).UTC(),
		},
	}
	msgs := newFakeSessionMessages()
	msgs.listResult["session-1"] = []message.Message{
		{ID: "message-1", SessionID: "session-1", Kind: message.KindUser},
	}
	dispatcher := newStubDispatcher()
	svc := NewSessionService(st, msgs, dispatcher)
	svc.nowFunc = timeNowStub

	err := svc.Restore(context.Background(), "session-1", dispatcher)
	require.NoError(t, err)
	require.Equal(t, "session-1", svc.GetSessionID())
	require.Equal(t, "session-1", msgs.lastListSessionID)

	event := dispatcher.lastEvent
	require.NotNil(t, event)

	payload, ok := event.Data().(SessionEvent)
	require.True(t, ok)
	require.Equal(t, StatusRestored, payload.Status)
	require.NotNil(t, payload.Session)
	require.Equal(t, "session-1", payload.Session.ID)
}

type fakeSessionStore struct {
	sessions []store.Session
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{}
}

func (s *fakeSessionStore) CreateSession(_ context.Context, params store.CreateSessionParams) (store.Session, error) {
	session := store.Session{
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
	}
	s.sessions = append([]store.Session{session}, s.sessions...)
	return session, nil
}

func (s *fakeSessionStore) ListSessions(_ context.Context) ([]store.Session, error) {
	return append([]store.Session(nil), s.sessions...), nil
}

func (s *fakeSessionStore) GetSession(_ context.Context, id string) (store.Session, error) {
	for _, session := range s.sessions {
		if session.ID == id {
			return session, nil
		}
	}

	return store.Session{}, store.ErrSessionNotFound
}

func (s *fakeSessionStore) UpdateSession(_ context.Context, params store.UpdateSessionParams) (store.Session, error) {
	for i := range s.sessions {
		if s.sessions[i].ID != params.ID {
			continue
		}

		if params.Title != "" {
			s.sessions[i].Title = params.Title
		}
		s.sessions[i].ParentSessionID = params.ParentSessionID
		s.sessions[i].MessageCount = params.MessageCount
		s.sessions[i].CompletionTokens = params.CompletionTokens
		s.sessions[i].CostMicros = params.CostMicros
		s.sessions[i].SummaryMessageID = params.SummaryMessageID
		s.sessions[i].TodosJSON = params.TodosJSON
		s.sessions[i].UpdatedAt = params.UpdatedAt

		return s.sessions[i], nil
	}

	return store.Session{}, store.ErrSessionNotFound
}

func (s *fakeSessionStore) DeleteSession(_ context.Context, id string) error {
	for i := range s.sessions {
		if s.sessions[i].ID != id {
			continue
		}

		s.sessions = append(s.sessions[:i], s.sessions[i+1:]...)
		return nil
	}

	return store.ErrSessionNotFound
}

type fakeSessionMessages struct {
	listResult          map[string][]message.Message
	createResult        message.Message
	lastListSessionID   string
	lastCreateSessionID string
	lastCreateParams    message.CreateMessageParams
}

func newFakeSessionMessages() *fakeSessionMessages {
	return &fakeSessionMessages{
		listResult: map[string][]message.Message{},
	}
}

func (s *fakeSessionMessages) ListMessages(_ context.Context, sessionID string, _ app.Dispatcher) ([]message.Message, error) {
	s.lastListSessionID = sessionID
	return append([]message.Message(nil), s.listResult[sessionID]...), nil
}

func (s *fakeSessionMessages) CreateMessage(_ context.Context, sessionID string, params message.CreateMessageParams, _ app.Dispatcher) (message.Message, error) {
	s.lastCreateSessionID = sessionID
	s.lastCreateParams = params
	msg := s.createResult
	msg.SessionID = sessionID
	if msg.ID == "" {
		msg.ID = "message-created"
	}
	return msg, nil
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

func timeNowStub() time.Time {
	return time.Unix(1710004000, 0).UTC()
}
