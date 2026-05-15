package app_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/YaHeii/agentGo/internal/app"
	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"github.com/stretchr/testify/require"
)

func TestServiceEnsureActiveSessionCreatesAndRestoresFirstSession(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	svc := newServiceWithDeps(sessions, newStubAgentService(), newStubToolService(), nil)

	err := svc.EnsureActiveSession(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"New Session"}, sessions.createdTitles)
	require.Equal(t, "session-created", sessions.restoredSessionID)
}

func TestServiceEnsureActiveSessionRestoresMostRecentSession(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	sessions.lastSessionID = "session-2"
	svc := newServiceWithDeps(sessions, newStubAgentService(), newStubToolService(), nil)

	err := svc.EnsureActiveSession(context.Background())
	require.NoError(t, err)
	require.Empty(t, sessions.createdTitles)
	require.Equal(t, "session-2", sessions.restoredSessionID)
}

func TestServiceCreateSessionDelegatesToSessionService(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	svc := newServiceWithDeps(sessions, newStubAgentService(), newStubToolService(), nil)

	sessionID, err := svc.CreateSession(context.Background(), "demo")
	require.NoError(t, err)
	require.Equal(t, "session-created", sessionID)
	require.Equal(t, []string{"demo"}, sessions.createdTitles)
}

func TestServiceDeleteSessionDelegatesToSessionService(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	svc := newServiceWithDeps(sessions, newStubAgentService(), newStubToolService(), nil)

	err := svc.DeleteSession(context.Background(), "session-1")
	require.NoError(t, err)
	require.Equal(t, []string{"session-1"}, sessions.deletedSessionIDs)
}

func TestServiceCreateMessageDelegatesToSessionService(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	sessions.createMessageResult = messagecontract.Message{
		ID:        "message-1",
		SessionID: "session-1",
		Kind:      messagecontract.KindUser,
	}
	svc := newServiceWithDeps(sessions, newStubAgentService(), newStubToolService(), nil)

	got, err := svc.CreateMessage(context.Background(), messagecontract.CreateMessageParams{
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
	require.Equal(t, sessions.createMessageResult, got)
	require.Equal(t, "session-1", sessions.lastCreateMessageParams.SessionID)
	require.Equal(t, messagecontract.KindUser, sessions.lastCreateMessageParams.Kind)
	require.Equal(t, "hello", sessions.lastCreateMessageParams.Parts[0].Text)
}

func TestServiceListSessionsDelegatesToSessionService(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	sessions.listResult = []sessioncontract.Session{
		{ID: "session-1", Title: "first"},
		{ID: "session-2", Title: "second"},
	}
	svc := newServiceWithDeps(sessions, newStubAgentService(), newStubToolService(), nil)

	got, err := svc.ListSessions(context.Background())
	require.NoError(t, err)
	require.Equal(t, sessions.listResult, got)
}

func TestServiceRenameSessionDelegatesToSessionService(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	sessions.renameResult = sessioncontract.Session{
		ID:        "session-1",
		Title:     "renamed",
		UpdatedAt: time.Unix(1710005000, 0).UTC(),
	}
	svc := newServiceWithDeps(sessions, newStubAgentService(), newStubToolService(), nil)

	got, err := svc.RenameSession(context.Background(), "session-1", "renamed")
	require.NoError(t, err)
	require.Equal(t, sessions.renameResult, got)
	require.Equal(t, "session-1", sessions.lastRenameID)
	require.Equal(t, "renamed", sessions.lastRenameTitle)
}

func TestServiceSwitchSessionDelegatesToSessionService(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	svc := newServiceWithDeps(sessions, newStubAgentService(), newStubToolService(), nil)

	err := svc.SwitchSession(context.Background(), "session-1")
	require.NoError(t, err)
	require.Equal(t, "session-1", sessions.switchedSessionID)
}

func TestServiceListHistoryDelegatesToSessionService(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	sessions.listHistoryResult = []messagecontract.Message{
		{ID: "message-1", SessionID: "session-1", Kind: messagecontract.KindUser},
	}
	svc := newServiceWithDeps(sessions, newStubAgentService(), newStubToolService(), nil)

	got, err := svc.ListHistory(context.Background(), "session-1")
	require.NoError(t, err)
	require.Equal(t, sessions.listHistoryResult, got)
	require.Equal(t, "session-1", sessions.lastListHistorySessionID)
}

func TestServiceExposesSessionStateGetters(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	sessions.currentSessionID = "session-1"
	sessions.parentSessionID = "parent-1"
	svc := newServiceWithDeps(sessions, newStubAgentService(), newStubToolService(), nil)

	require.Equal(t, "session-1", svc.GetSessionID())
	require.Equal(t, "parent-1", svc.GetParentSessionID())
}

func TestServiceListToolsDelegatesToToolService(t *testing.T) {
	t.Parallel()

	tools := newStubToolService()
	tools.listResult = []toolcontract.Metadata{
		{
			Name:              "grep",
			Description:       "search files",
			Parameters:        json.RawMessage(`{"type":"object"}`),
			Enabled:           true,
			IsConcurrencySafe: true,
		},
	}
	svc := newServiceWithDeps(newStubSessionService(), newStubAgentService(), tools, nil)

	got := svc.ListTools(context.Background(), toolcontract.AttentionLevel)
	require.Equal(t, tools.listResult, got)
	require.Equal(t, 1, tools.listCalls)
	require.Equal(t, toolcontract.AttentionLevel, tools.lastListPermissionLevel)
}

func TestServiceCallToolsDelegatesToToolService(t *testing.T) {
	t.Parallel()

	tools := newStubToolService()
	tools.callResult = []toolcontract.ToolResult{
		{
			ToolCallID: "call-1",
			Name:       "grep",
			Status:     toolcontract.StatusSuccess,
			Content:    `{"matches":[]}`,
		},
	}
	svc := newServiceWithDeps(newStubSessionService(), newStubAgentService(), tools, nil)

	req := toolcontract.BatchRequest{
		Calls: []toolcontract.ToolCallRequest{{
			ToolCallID:      "call-1",
			Name:            "grep",
			Arguments:       json.RawMessage(`{"pattern":"go"}`),
			PermissionLevel: toolcontract.SafeLevel,
			Context:         toolcontract.ToolCallContext{WorkingDir: "/workspace"},
		}},
	}
	got, err := svc.CallTools(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, tools.callResult, got)
	require.Equal(t, req, tools.lastCallReq)
}

func TestServiceRunQueryDelegatesPromptToAgent(t *testing.T) {
	t.Parallel()

	agentSvc := newStubAgentService()
	svc := newServiceWithDeps(newStubSessionService(), agentSvc, newStubToolService(), nil)

	err := svc.RunQuery(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Equal(t, "session-1", agentSvc.lastSessionID)
	require.Equal(t, "hello", agentSvc.lastPrompt)
}

func TestDispatcherPublishesEventsToSubscribers(t *testing.T) {
	t.Parallel()

	dispatcher := app.NewDispatcher(4)
	events := dispatcher.Subscribe(context.Background())

	want := fakeEvent{name: app.EventSession, payload: "session-created"}
	dispatcher.Dispatch(want)

	got := <-events
	require.Equal(t, want, got)
}

type stubSessionService struct {
	lastSessionID            string
	currentSessionID         string
	parentSessionID          string
	getLastErr               error
	createErr                error
	createMessageErr         error
	listErr                  error
	renameErr                error
	restoreErr               error
	switchErr                error
	deleteErr                error
	createdTitles            []string
	restoredSessionID        string
	deletedSessionIDs        []string
	listResult               []sessioncontract.Session
	renameResult             sessioncontract.Session
	lastRenameID             string
	lastRenameTitle          string
	switchedSessionID        string
	listHistoryResult        []messagecontract.Message
	lastListHistorySessionID string
	lastCreateMessageParams  messagecontract.CreateMessageParams
	createMessageResult      messagecontract.Message
}

func newStubSessionService() *stubSessionService {
	return &stubSessionService{}
}

func (s *stubSessionService) Create(_ context.Context, title string, _ app.Dispatcher) (string, error) {
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
		return "", sessioncontract.ErrSessionNotFound
	}
	return s.lastSessionID, nil
}

func (s *stubSessionService) List(_ context.Context) ([]sessioncontract.Session, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]sessioncontract.Session(nil), s.listResult...), nil
}

func (s *stubSessionService) Rename(_ context.Context, id string, title string, _ app.Dispatcher) (sessioncontract.Session, error) {
	s.lastRenameID = id
	s.lastRenameTitle = title
	if s.renameErr != nil {
		return sessioncontract.Session{}, s.renameErr
	}
	return s.renameResult, nil
}

func (s *stubSessionService) Restore(_ context.Context, sessionID string, _ app.Dispatcher) error {
	s.restoredSessionID = sessionID
	return s.restoreErr
}

func (s *stubSessionService) SwitchSession(_ context.Context, sessionID string, _ app.Dispatcher) error {
	s.switchedSessionID = sessionID
	return s.switchErr
}

func (s *stubSessionService) GetSessionID() string {
	return s.currentSessionID
}

func (s *stubSessionService) GetParentSessionID() string {
	return s.parentSessionID
}

func (s *stubSessionService) ListHistory(_ context.Context, sessionID string, _ app.Dispatcher) ([]messagecontract.Message, error) {
	s.lastListHistorySessionID = sessionID
	return append([]messagecontract.Message(nil), s.listHistoryResult...), nil
}

func (s *stubSessionService) Delete(_ context.Context, sessionID string, _ app.Dispatcher) error {
	s.deletedSessionIDs = append(s.deletedSessionIDs, sessionID)
	return s.deleteErr
}

func (s *stubSessionService) CreateMessage(_ context.Context, params messagecontract.CreateMessageParams, _ app.Dispatcher) (messagecontract.Message, error) {
	s.lastCreateMessageParams = params
	if s.createMessageErr != nil {
		return messagecontract.Message{}, s.createMessageErr
	}
	return s.createMessageResult, nil
}

type stubAgentService struct {
	runErr        error
	lastSessionID string
	lastPrompt    string
}

func newStubAgentService() *stubAgentService {
	return &stubAgentService{}
}

func (s *stubAgentService) RunQuery(_ context.Context, sessionID string, prompt string) error {
	s.lastSessionID = sessionID
	s.lastPrompt = prompt
	return s.runErr
}

type stubToolService struct {
	listCalls               int
	lastListPermissionLevel toolcontract.SecurityLevel
	listResult              []toolcontract.Metadata
	callErr                 error
	lastCallReq             toolcontract.BatchRequest
	callResult              []toolcontract.ToolResult
}

func newStubToolService() *stubToolService {
	return &stubToolService{}
}

func (s *stubToolService) ListTools(_ context.Context, permissionLevel toolcontract.SecurityLevel) []toolcontract.Metadata {
	s.listCalls++
	s.lastListPermissionLevel = permissionLevel
	return append([]toolcontract.Metadata(nil), s.listResult...)
}

func (s *stubToolService) Call(_ context.Context, req toolcontract.BatchRequest) ([]toolcontract.ToolResult, error) {
	s.lastCallReq = req
	if s.callErr != nil {
		return nil, s.callErr
	}
	return append([]toolcontract.ToolResult(nil), s.callResult...), nil
}

type fakeEvent struct {
	name    app.EventType
	payload any
}

func (e fakeEvent) Type() app.EventType {
	return e.name
}

func (e fakeEvent) Data() any {
	return e.payload
}

type sessionStore interface {
	Create(ctx context.Context, title string, d app.Dispatcher) (string, error)
	GetLast(ctx context.Context) (string, error)
	List(ctx context.Context) ([]sessioncontract.Session, error)
	Rename(ctx context.Context, id string, title string, d app.Dispatcher) (sessioncontract.Session, error)
	Restore(ctx context.Context, sessionID string, d app.Dispatcher) error
	SwitchSession(ctx context.Context, sessionID string, d app.Dispatcher) error
	GetSessionID() string
	GetParentSessionID() string
	Delete(ctx context.Context, id string, d app.Dispatcher) error
	ListHistory(ctx context.Context, sessionID string, d app.Dispatcher) ([]messagecontract.Message, error)
	CreateMessage(ctx context.Context, params messagecontract.CreateMessageParams, d app.Dispatcher) (messagecontract.Message, error)
}

type agentStore interface {
	RunQuery(ctx context.Context, sessionID string, prompt string) error
}

type toolStore interface {
	ListTools(ctx context.Context, permissionLevel toolcontract.SecurityLevel) []toolcontract.Metadata
	Call(ctx context.Context, req toolcontract.BatchRequest) ([]toolcontract.ToolResult, error)
}

func newServiceWithDeps(sessions sessionStore, agent agentStore, tools toolStore, dispatcher app.Dispatcher) *app.APPService {
	if dispatcher == nil {
		dispatcher = app.NewDispatcher(16)
	}

	return app.NewService(sessions, agent, tools, dispatcher)
}
