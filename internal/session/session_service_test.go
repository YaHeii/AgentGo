package session

import (
	"context"
	"testing"
	"time"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/lifecycle"
	"github.com/YaHeii/agentGo/internal/message"
	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
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

	got, err := svc.Get(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, sessioncontract.Session{
		ID:        sessionID,
		Title:     "New Session",
		TodosJSON: "[]",
		CreatedAt: timeNowStub().UTC(),
		UpdatedAt: timeNowStub().UTC(),
	}, got)

	event := dispatcher.lastEvent
	require.NotNil(t, event)
	require.Equal(t, app.EventSession, event.Type())
	require.Equal(t, app.EventSessionCreated, event.Name())

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
	st.sessions = []sessioncontract.Session{
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
	require.Equal(t, sessioncontract.Session{
		ID:        "session-1",
		Title:     "new",
		TodosJSON: "[]",
		CreatedAt: time.Unix(1710000000, 0).UTC(),
		UpdatedAt: timeNowStub().UTC(),
	}, session)
	require.Equal(t, "new", session.Title)

	event := dispatcher.lastEvent
	require.NotNil(t, event)
	require.Equal(t, app.EventSession, event.Type())
	require.Equal(t, app.EventSessionUpdated, event.Name())

	payload, ok := event.Data().(SessionEvent)
	require.True(t, ok)
	require.Equal(t, StatusUpdated, payload.Status)
	require.NotNil(t, payload.Session)
	require.Equal(t, "new", payload.Session.Title)
}

func TestSessionServiceDeletePublishesDeletedEvent(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	st.sessions = []sessioncontract.Session{
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

	err := svc.Delete(context.Background(), "session-1", dispatcher)
	require.NoError(t, err)

	event := dispatcher.lastEvent
	require.NotNil(t, event)
	require.Equal(t, app.EventSession, event.Type())
	require.Equal(t, app.EventSessionDeleted, event.Name())

	payload, ok := event.Data().(SessionEvent)
	require.True(t, ok)
	require.Equal(t, StatusDeleted, payload.Status)
	require.Nil(t, payload.Session)
}

func TestSessionServiceSaveUpdatesSessionTodosJSON(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	st.sessions = []sessioncontract.Session{
		{
			ID:        "session-1",
			Title:     "chat",
			TodosJSON: "[]",
			CreatedAt: time.Unix(1710000000, 0).UTC(),
			UpdatedAt: time.Unix(1710000000, 0).UTC(),
		},
	}
	svc := NewSessionService(st, newFakeSessionMessages(), newStubDispatcher())
	svc.nowFunc = timeNowStub

	saved, err := svc.Save(context.Background(), sessioncontract.Session{
		ID:        "session-1",
		Title:     "chat",
		TodosJSON: `[{"content":"wire todos","status":"completed"}]`,
		CreatedAt: time.Unix(1710000000, 0).UTC(),
	})

	require.NoError(t, err)
	require.Equal(t, "session-1", saved.ID)
	require.JSONEq(t, `[{"content":"wire todos","status":"completed"}]`, saved.TodosJSON)
	require.Equal(t, timeNowStub().UTC(), saved.UpdatedAt)
}

func TestSessionServiceGetLastUsesUpdatedAtOrdering(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	st.sessions = []sessioncontract.Session{
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

func TestSessionServiceListReturnsContractSessions(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	st.sessions = []sessioncontract.Session{
		{
			ID:        "session-2",
			Title:     "latest",
			CreatedAt: time.Unix(1710000000, 0).UTC(),
			UpdatedAt: time.Unix(1710000500, 0).UTC(),
		},
	}
	svc := NewSessionService(st, newFakeSessionMessages(), newStubDispatcher())
	svc.nowFunc = timeNowStub

	got, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Equal(t, []sessioncontract.Session{
		{
			ID:        "session-2",
			Title:     "latest",
			CreatedAt: time.Unix(1710000000, 0).UTC(),
			UpdatedAt: time.Unix(1710000500, 0).UTC(),
		},
	}, got)
}

func TestSessionServiceListHistoryDelegatesToMessageDependency(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	msgs := newFakeSessionMessages()
	dispatcher := newStubDispatcher()
	msgs.listResult["session-1"] = []messagecontract.Message{
		{ID: "message-1", SessionID: "session-1", Kind: messagecontract.KindUser},
	}
	svc := NewSessionService(st, msgs, dispatcher)
	svc.nowFunc = timeNowStub

	got, err := svc.ListHistory(context.Background(), "session-1", dispatcher)
	require.NoError(t, err)
	require.Equal(t, []messagecontract.Message{{ID: "message-1", SessionID: "session-1", Kind: messagecontract.KindUser}}, got)
	require.Equal(t, "session-1", msgs.lastListSessionID)
}

func TestSessionServiceCreateMessageRoutesThroughMessageDependency(t *testing.T) {
	t.Parallel()

	st := newFakeSessionStore()
	msgs := newFakeSessionMessages()
	dispatcher := newStubDispatcher()
	msgs.createResult = messagecontract.Message{ID: "message-1", SessionID: "session-1", Kind: messagecontract.KindAssistant}
	svc := NewSessionService(st, msgs, dispatcher)
	svc.nowFunc = timeNowStub

	got, err := svc.CreateMessage(context.Background(), messagecontract.CreateMessageParams{
		SessionID: "session-1",
		Kind:      messagecontract.KindAssistant,
	}, dispatcher)
	require.NoError(t, err)
	require.Equal(t, msgs.createResult, got)
	require.Equal(t, "session-1", msgs.lastCreateParams.SessionID)
	require.Equal(t, messagecontract.KindAssistant, msgs.lastCreateParams.Kind)

	event := dispatcher.lastEvent
	require.NotNil(t, event)
	require.Equal(t, app.EventMessage, event.Type())
	require.Equal(t, app.EventMessageCreated, event.Name())

	payload, ok := event.Data().(message.MessageEvent)
	require.True(t, ok)
	require.Equal(t, message.StatusPending, payload.Status)
	require.NotNil(t, payload.Message)
	require.Equal(t, got.ID, payload.Message.ID)
}

func TestSessionServiceSwitchSessionUpdatesStateAndPublishesEvent(t *testing.T) {
	lifecycle.State = &lifecycle.GlobalState{}
	t.Cleanup(func() {
		lifecycle.State = nil
	})

	st := newFakeSessionStore()
	st.sessions = []sessioncontract.Session{
		{
			ID: "session-1",
		},
	}
	dispatcher := newStubDispatcher()
	svc := NewSessionService(st, newFakeSessionMessages(), dispatcher)
	svc.nowFunc = timeNowStub

	err := svc.SwitchSession(context.Background(), "session-1", dispatcher)
	require.NoError(t, err)
	require.Equal(t, "session-1", svc.GetSessionID())
	require.Equal(t, "session-1", lifecycle.State.SessionID)

	event := dispatcher.lastEvent
	require.NotNil(t, event)
	require.Equal(t, app.EventSession, event.Type())

	payload, ok := event.Data().(SessionEvent)
	require.True(t, ok)
	require.Equal(t, StatusSwitched, payload.Status)
	require.NotNil(t, payload.Session)
	require.Equal(t, "session-1", payload.Session.ID)
	require.Equal(t, app.EventSessionSwitched, event.Name())
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
	lifecycle.State = &lifecycle.GlobalState{}
	t.Cleanup(func() {
		lifecycle.State = nil
	})

	st := newFakeSessionStore()
	st.sessions = []sessioncontract.Session{
		{
			ID:        "session-1",
			Title:     "restored",
			TodosJSON: "[]",
			CreatedAt: time.Unix(1710000000, 0).UTC(),
			UpdatedAt: time.Unix(1710000100, 0).UTC(),
		},
	}
	msgs := newFakeSessionMessages()
	msgs.listResult["session-1"] = []messagecontract.Message{
		{ID: "message-1", SessionID: "session-1", Kind: messagecontract.KindUser},
	}
	dispatcher := newStubDispatcher()
	svc := NewSessionService(st, msgs, dispatcher)
	svc.nowFunc = timeNowStub

	err := svc.Restore(context.Background(), "session-1", dispatcher)
	require.NoError(t, err)
	require.Equal(t, "session-1", svc.GetSessionID())
	require.Equal(t, "session-1", msgs.lastListSessionID)
	require.Equal(t, "session-1", lifecycle.State.SessionID)

	event := dispatcher.lastEvent
	require.NotNil(t, event)
	require.Equal(t, app.EventSession, event.Type())

	payload, ok := event.Data().(SessionEvent)
	require.True(t, ok)
	require.Equal(t, StatusRestored, payload.Status)
	require.NotNil(t, payload.Session)
	require.Equal(t, "session-1", payload.Session.ID)
	require.Equal(t, app.EventSessionRestored, event.Name())
}

type fakeSessionStore struct {
	sessions []sessioncontract.Session
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{}
}

func (s *fakeSessionStore) CreateSession(_ context.Context, params sessioncontract.CreateSessionParams) (sessioncontract.Session, error) {
	session := sessioncontract.Session{
		ID:               params.ID,
		Title:            params.Title,
		MessageCount:     params.MessageCount,
		CompletionTokens: params.CompletionTokens,
		CostMicros:       params.CostMicros,
		SummaryMessageID: params.SummaryMessageID,
		TodosJSON:        params.TodosJSON,
		CreatedAt:        params.CreatedAt,
		UpdatedAt:        params.UpdatedAt,
	}
	s.sessions = append([]sessioncontract.Session{session}, s.sessions...)
	return session, nil
}

func (s *fakeSessionStore) ListSessions(_ context.Context) ([]sessioncontract.Session, error) {
	return append([]sessioncontract.Session(nil), s.sessions...), nil
}

func (s *fakeSessionStore) GetSession(_ context.Context, id string) (sessioncontract.Session, error) {
	for _, session := range s.sessions {
		if session.ID == id {
			return session, nil
		}
	}

	return sessioncontract.Session{}, sessioncontract.ErrSessionNotFound
}

func (s *fakeSessionStore) UpdateSession(_ context.Context, params sessioncontract.UpdateSessionParams) (sessioncontract.Session, error) {
	for i := range s.sessions {
		if s.sessions[i].ID != params.ID {
			continue
		}

		if params.Title != "" {
			s.sessions[i].Title = params.Title
		}
		s.sessions[i].MessageCount = params.MessageCount
		s.sessions[i].CompletionTokens = params.CompletionTokens
		s.sessions[i].CostMicros = params.CostMicros
		s.sessions[i].SummaryMessageID = params.SummaryMessageID
		s.sessions[i].TodosJSON = params.TodosJSON
		s.sessions[i].UpdatedAt = params.UpdatedAt

		return s.sessions[i], nil
	}

	return sessioncontract.Session{}, sessioncontract.ErrSessionNotFound
}

func (s *fakeSessionStore) DeleteSession(_ context.Context, id string) error {
	for i := range s.sessions {
		if s.sessions[i].ID != id {
			continue
		}

		s.sessions = append(s.sessions[:i], s.sessions[i+1:]...)
		return nil
	}

	return sessioncontract.ErrSessionNotFound
}

type fakeSessionMessages struct {
	listResult        map[string][]messagecontract.Message
	createResult      messagecontract.Message
	lastListSessionID string
	lastCreateParams  messagecontract.CreateMessageParams
}

func newFakeSessionMessages() *fakeSessionMessages {
	return &fakeSessionMessages{
		listResult: map[string][]messagecontract.Message{},
	}
}

func (s *fakeSessionMessages) ListMessages(_ context.Context, sessionID string) ([]messagecontract.Message, error) {
	s.lastListSessionID = sessionID
	return append([]messagecontract.Message(nil), s.listResult[sessionID]...), nil
}

func (s *fakeSessionMessages) CreateMessage(_ context.Context, params messagecontract.CreateMessageParams) (messagecontract.Message, error) {
	s.lastCreateParams = params
	msg := s.createResult
	msg.SessionID = params.SessionID
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
