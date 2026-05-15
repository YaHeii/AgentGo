package app_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/YaHeii/agentGo/internal/tool"
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
	sessions.createMessageResult = message.Message{
		ID:        "message-1",
		SessionID: "session-1",
		Kind:      message.KindUser,
	}
	svc := newServiceWithDeps(sessions, newStubAgentService(), newStubToolService(), nil)

	got, err := svc.CreateMessage(context.Background(), message.CreateMessageParams{
		SessionID: "session-1",
		Kind:      message.KindUser,
		Parts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: "hello",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, sessions.createMessageResult, got)
	require.Equal(t, "session-1", sessions.lastCreateMessageParams.SessionID)
	require.Equal(t, message.KindUser, sessions.lastCreateMessageParams.Kind)
	require.Equal(t, "hello", sessions.lastCreateMessageParams.Parts[0].Text)
}

func TestServiceListSessionsDelegatesToSessionService(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	sessions.listResult = []store.Session{
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
	sessions.renameResult = store.Session{
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
	sessions.listHistoryResult = []message.Message{
		{ID: "message-1", SessionID: "session-1", Kind: message.KindUser},
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
	tools.listResult = []tool.Metadata{
		{
			Name:              "grep",
			Description:       "search files",
			Parameters:        json.RawMessage(`{"type":"object"}`),
			Enabled:           true,
			IsConcurrencySafe: true,
		},
	}
	svc := newServiceWithDeps(newStubSessionService(), newStubAgentService(), tools, nil)

	got := svc.ListTools(context.Background(), tool.AttentionLevel)
	require.Equal(t, tools.listResult, got)
	require.Equal(t, 1, tools.listCalls)
	require.Equal(t, tool.AttentionLevel, tools.lastListPermissionLevel)
}

func TestServiceCallToolsDelegatesToToolService(t *testing.T) {
	t.Parallel()

	tools := newStubToolService()
	tools.callResult = []tool.ToolResult{
		{
			ToolCallID: "call-1",
			Name:       "grep",
			Status:     tool.StatusSuccess,
			Content:    `{"matches":[]}`,
		},
	}
	svc := newServiceWithDeps(newStubSessionService(), newStubAgentService(), tools, nil)

	req := tool.NewBatchRequest(
		tool.NewToolCallRequest(
			"call-1",
			"grep",
			json.RawMessage(`{"pattern":"go"}`),
			tool.SafeLevel,
			tool.ToolCallContext{WorkingDir: "/workspace"},
		),
	)
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
	listResult               []store.Session
	renameResult             store.Session
	lastRenameID             string
	lastRenameTitle          string
	switchedSessionID        string
	listHistoryResult        []message.Message
	lastListHistorySessionID string
	lastCreateMessageParams  message.CreateMessageParams
	createMessageResult      message.Message
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
		return "", store.ErrSessionNotFound
	}
	return s.lastSessionID, nil
}

func (s *stubSessionService) List(_ context.Context) ([]store.Session, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]store.Session(nil), s.listResult...), nil
}

func (s *stubSessionService) Rename(_ context.Context, id string, title string, _ app.Dispatcher) (store.Session, error) {
	s.lastRenameID = id
	s.lastRenameTitle = title
	if s.renameErr != nil {
		return store.Session{}, s.renameErr
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

func (s *stubSessionService) ListHistory(_ context.Context, sessionID string, _ app.Dispatcher) ([]message.Message, error) {
	s.lastListHistorySessionID = sessionID
	return append([]message.Message(nil), s.listHistoryResult...), nil
}

func (s *stubSessionService) Delete(_ context.Context, sessionID string, _ app.Dispatcher) error {
	s.deletedSessionIDs = append(s.deletedSessionIDs, sessionID)
	return s.deleteErr
}

func (s *stubSessionService) CreateMessage(_ context.Context, params message.CreateMessageParams, _ app.Dispatcher) (message.Message, error) {
	s.lastCreateMessageParams = params
	if s.createMessageErr != nil {
		return message.Message{}, s.createMessageErr
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
	lastListPermissionLevel tool.SecurityLevel
	listResult              []tool.Metadata
	callErr                 error
	lastCallReq             tool.BatchRequest
	callResult              []tool.ToolResult
}

func newStubToolService() *stubToolService {
	return &stubToolService{}
}

func (s *stubToolService) ListTools(_ context.Context, permissionLevel tool.SecurityLevel) []tool.Metadata {
	s.listCalls++
	s.lastListPermissionLevel = permissionLevel
	return append([]tool.Metadata(nil), s.listResult...)
}

func (s *stubToolService) Call(_ context.Context, req tool.BatchRequest) ([]tool.ToolResult, error) {
	s.lastCallReq = req
	if s.callErr != nil {
		return nil, s.callErr
	}
	return append([]tool.ToolResult(nil), s.callResult...), nil
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
	List(ctx context.Context) ([]store.Session, error)
	Rename(ctx context.Context, id string, title string, d app.Dispatcher) (store.Session, error)
	Restore(ctx context.Context, sessionID string, d app.Dispatcher) error
	SwitchSession(ctx context.Context, sessionID string, d app.Dispatcher) error
	GetSessionID() string
	GetParentSessionID() string
	Delete(ctx context.Context, id string, d app.Dispatcher) error
	ListHistory(ctx context.Context, sessionID string, d app.Dispatcher) ([]message.Message, error)
	CreateMessage(ctx context.Context, params message.CreateMessageParams, d app.Dispatcher) (message.Message, error)
}

type agentStore interface {
	RunQuery(ctx context.Context, sessionID string, prompt string) error
}

type toolStore interface {
	ListTools(ctx context.Context, permissionLevel tool.SecurityLevel) []tool.Metadata
	Call(ctx context.Context, req tool.BatchRequest) ([]tool.ToolResult, error)
}

func newServiceWithDeps(sessions sessionStore, agent agentStore, tools toolStore, dispatcher app.Dispatcher) *app.APPService {
	if dispatcher == nil {
		dispatcher = app.NewDispatcher(16)
	}
	
	return app.NewService(sessions, agent, tools, dispatcher)
}
