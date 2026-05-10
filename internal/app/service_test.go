package app

import (
	"context"
	"testing"

	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/stretchr/testify/require"
)

func TestServiceEnsureActiveSessionCreatesAndRestoresFirstSession(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	svc := newServiceWithDeps(sessions, newStubAgentService(), nil)

	err := svc.EnsureActiveSession(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"New Session"}, sessions.createdTitles)
	require.Equal(t, "session-created", sessions.restoredSessionID)
}

func TestServiceEnsureActiveSessionRestoresMostRecentSession(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	sessions.lastSession = store.Session{
		ID:    "session-2",
		Title: "latest",
	}
	svc := newServiceWithDeps(sessions, newStubAgentService(), nil)

	err := svc.EnsureActiveSession(context.Background())
	require.NoError(t, err)
	require.Empty(t, sessions.createdTitles)
	require.Equal(t, "session-2", sessions.restoredSessionID)
}

func TestServiceCreateSessionDelegatesToSessionService(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	svc := newServiceWithDeps(sessions, newStubAgentService(), nil)

	err := svc.CreateSession(context.Background(), "demo")
	require.NoError(t, err)
	require.Equal(t, []string{"demo"}, sessions.createdTitles)
}

func TestServiceDeleteSessionDelegatesToSessionService(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	svc := newServiceWithDeps(sessions, newStubAgentService(), nil)

	err := svc.DeleteSession(context.Background(), "session-1")
	require.NoError(t, err)
	require.Equal(t, []string{"session-1"}, sessions.deletedSessionIDs)
}

func TestServiceSendMessageDelegatesPromptToAgent(t *testing.T) {
	t.Parallel()

	agentSvc := newStubAgentService()
	svc := newServiceWithDeps(newStubSessionService(), agentSvc, nil)

	err := svc.SendMessage(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Equal(t, "session-1", agentSvc.lastSessionID)
	require.Equal(t, "hello", agentSvc.lastPrompt)
}

func TestServiceExposesUnifiedEventStream(t *testing.T) {
	t.Parallel()

	events := bus.NewBus[Event](4)
	svc := newServiceWithDeps(newStubSessionService(), newStubAgentService(), events)

	want := fakeEvent{name: EventTypeSession}
	events.Publish(want)

	got := <-svc.Events()
	require.Equal(t, want, got)
}

type stubSessionService struct {
	lastSession       store.Session
	getLastErr        error
	createErr         error
	restoreErr        error
	deleteErr         error
	createdTitles     []string
	restoredSessionID string
	deletedSessionIDs []string
}

func newStubSessionService() *stubSessionService {
	return &stubSessionService{}
}

func (s *stubSessionService) Create(_ context.Context, title string) (store.Session, error) {
	s.createdTitles = append(s.createdTitles, title)
	if s.createErr != nil {
		return store.Session{}, s.createErr
	}

	created := store.Session{
		ID:    "session-created",
		Title: title,
	}
	s.lastSession = created
	return created, nil
}

func (s *stubSessionService) GetLast(_ context.Context) (store.Session, error) {
	if s.getLastErr != nil {
		return store.Session{}, s.getLastErr
	}
	if s.lastSession.ID == "" {
		return store.Session{}, store.ErrSessionNotFound
	}
	return s.lastSession, nil
}

func (s *stubSessionService) Restore(_ context.Context, sessionID string) error {
	s.restoredSessionID = sessionID
	return s.restoreErr
}

func (s *stubSessionService) Delete(_ context.Context, sessionID string) error {
	s.deletedSessionIDs = append(s.deletedSessionIDs, sessionID)
	return s.deleteErr
}

type stubAgentService struct {
	runErr        error
	lastSessionID string
	lastPrompt    string
}

func newStubAgentService() *stubAgentService {
	return &stubAgentService{}
}

func (s *stubAgentService) RunPrompt(_ context.Context, sessionID string, prompt string) error {
	s.lastSessionID = sessionID
	s.lastPrompt = prompt
	return s.runErr
}

type fakeEvent struct {
	name EventType
}

func (e fakeEvent) Type() EventType {
	return e.name
}

func newServiceWithDeps(sessions sessionStore, agent agentStore, eventBus bus.Bus[Event]) *APPService {
	if eventBus == nil {
		eventBus = bus.NewBus[Event](16)
	}

	return &APPService{
		sessions: sessions,
		agent:    agent,
		bus:      eventBus,
		events:   eventBus.Subscribe(context.Background()),
	}
}
