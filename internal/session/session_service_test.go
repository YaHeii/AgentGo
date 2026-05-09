package session

import (
	"context"
	"testing"
	"time"

	"github.com/YaHeii/agentGo/internal/store"
	"github.com/stretchr/testify/require"
)

func TestSessionServiceCreatePublishesCreatedEvent(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := NewSessionService(st, timeNowStub)

	session, err := svc.Create(context.Background(), "New Session")
	require.NoError(t, err)
	require.NotEmpty(t, session.ID)
	require.Equal(t, "New Session", session.Title)

	event := <-svc.Events()
	require.IsType(t, SessionCreatedEvent{}, event)
	require.Equal(t, session.ID, event.(SessionCreatedEvent).Session.ID)
}

func TestSessionServiceGetLastReturnsNotFoundWhenEmpty(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	svc := NewSessionService(st, timeNowStub)

	_, err := svc.GetLast(context.Background())
	require.ErrorIs(t, err, ErrSessionNotFound)
}

func TestSessionServiceRenameUpdatesSessionAndPublishesUpdatedEvent(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.sessions = []store.Session{
		{
			ID:        "session-1",
			Title:     "old",
			TodosJSON: "[]",
			CreatedAt: time.Unix(1710000000, 0).UTC(),
			UpdatedAt: time.Unix(1710000000, 0).UTC(),
		},
	}
	svc := NewSessionService(st, timeNowStub)

	session, err := svc.Rename(context.Background(), "session-1", "new")
	require.NoError(t, err)
	require.Equal(t, "new", session.Title)

	event := <-svc.Events()
	require.IsType(t, SessionUpdatedEvent{}, event)
	require.Equal(t, "new", event.(SessionUpdatedEvent).Session.Title)
}

func TestSessionServiceGetLastUsesUpdatedAtOrdering(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
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
	svc := NewSessionService(st, timeNowStub)

	got, err := svc.GetLast(context.Background())
	require.NoError(t, err)
	require.Equal(t, "session-2", got.ID)
}

func timeNowStub() time.Time {
	return time.Unix(1710004000, 0).UTC()
}

type fakeStore struct {
	sessions []store.Session
}

func newFakeStore() *fakeStore {
	return &fakeStore{}
}

func (s *fakeStore) CreateSession(_ context.Context, params store.CreateSessionParams) (store.Session, error) {
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

func (s *fakeStore) ListSessions(_ context.Context) ([]store.Session, error) {
	return append([]store.Session(nil), s.sessions...), nil
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

func (s *fakeStore) DeleteSession(_ context.Context, id string) error {
	for i := range s.sessions {
		if s.sessions[i].ID != id {
			continue
		}

		s.sessions = append(s.sessions[:i], s.sessions[i+1:]...)
		return nil
	}

	return store.ErrSessionNotFound
}
