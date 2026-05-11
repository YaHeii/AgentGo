package app

import (
	"context"
	"testing"

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
	sessions.lastSessionID = "session-2"
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

	sessionID, err := svc.CreateSession(context.Background(), "demo")
	require.NoError(t, err)
	require.Equal(t, "session-created", sessionID)
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

func TestDispatcherPublishesEventsToSubscribers(t *testing.T) {
	t.Parallel()

	dispatcher := NewDispatcher(4)
	events := dispatcher.Subscribe(context.Background())

	want := fakeEvent{name: EventSession, payload: "session-created"}
	dispatcher.Dispatch(want)

	got := <-events
	require.Equal(t, want, got)
}

type stubSessionService struct {
	lastSessionID     string
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

func (s *stubSessionService) Create(_ context.Context, title string, _ Dispatcher) (string, error) {
	s.createdTitles = append(s.createdTitles, title)
	if s.createErr != nil {
		return "", s.createErr
	}

	s.lastSessionID = "session-created"
	return s.lastSessionID, nil
}

func (s *stubSessionService) GetLast(_ context.Context) (string, error) {
	if s.getLastErr != nil {
		return "", s.getLastErr
	}
	if s.lastSessionID == "" {
		return "", store.ErrSessionNotFound
	}
	return s.lastSessionID, nil
}

func (s *stubSessionService) Restore(_ context.Context, sessionID string, _ Dispatcher) error {
	s.restoredSessionID = sessionID
	return s.restoreErr
}

func (s *stubSessionService) Delete(_ context.Context, sessionID string, _ Dispatcher) error {
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
	name    EventType
	payload any
}

func (e fakeEvent) Type() EventType {
	return e.name
}

func (e fakeEvent) Data() any {
	return e.payload
}

func newServiceWithDeps(sessions sessionStore, agent agentStore, dispatcher Dispatcher) *APPService {
	if dispatcher == nil {
		dispatcher = NewDispatcher(16)
	}

	return &APPService{
		sessions:   sessions,
		agent:      agent,
		dispatcher: dispatcher,
	}
}
