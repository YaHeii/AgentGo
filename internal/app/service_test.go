package app

import (
	"context"
	"testing"
	"time"

	"github.com/YaHeii/agentGo/internal/agent"
	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/session"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/stretchr/testify/require"
)

func TestServiceEnsureActiveSessionCreatesFirstSessionAndPublishesBootstrapEvents(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	svc := newServiceWithDeps(sessions, &stubMessageService{}, newStubQueryRunner(), timeNowStub)

	session, err := svc.EnsureActiveSession(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, session.ID)
	require.Len(t, sessions.createdTitles, 1)

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

	sessions := newStubSessionService()
	sessions.listResult = []store.Session{
		{
			ID:        "session-2",
			Title:     "latest",
			CreatedAt: time.Unix(1710000200, 0).UTC(),
			UpdatedAt: time.Unix(1710000200, 0).UTC(),
		},
		{
			ID:        "session-1",
			Title:     "older",
			CreatedAt: time.Unix(1710000000, 0).UTC(),
			UpdatedAt: time.Unix(1710000000, 0).UTC(),
		},
	}
	msgSvc := &stubMessageService{
		listResult: []message.Message{
			messageRecord("u1", message.KindUser, "hello"),
			messageRecord("a1", message.KindAssistant, "world"),
		},
	}
	svc := newServiceWithDeps(sessions, msgSvc, newStubQueryRunner(), timeNowStub)

	session, err := svc.EnsureActiveSession(context.Background())
	require.NoError(t, err)
	require.Equal(t, "session-2", session.ID)
	require.Equal(t, "session-2", msgSvc.lastListSessionID)

	<-svc.Events()
	hydrated := (<-svc.Events()).(ConversationHydratedEvent)
	require.Equal(t, "session-2", hydrated.SessionID)
	require.Len(t, hydrated.Messages, 2)
	require.Equal(t, "hello", hydrated.Messages[0].Content)
	require.Equal(t, "world", hydrated.Messages[1].Content)
}

func TestServiceForwardsAgentEventsToUnifiedAppEventStream(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	msgSvc := &stubMessageService{}
	query := newStubQueryRunner()
	svc := newServiceWithDeps(sessions, msgSvc, query, timeNowStub)

	query.publish(agent.QueryCompletedEvent{
		SessionID:          "session-1",
		UserMessageID:      "user-1",
		AssistantMessageID: "assistant-1",
	})

	event := <-svc.Events()
	completed, ok := event.(QueryCompletedEvent)
	require.True(t, ok)
	require.Equal(t, "session-1", completed.SessionID)
	require.Equal(t, "user-1", completed.UserMessageID)
	require.Equal(t, "assistant-1", completed.AssistantMessageID)
}

func TestServiceSendMessageDelegatesToMessageService(t *testing.T) {
	t.Parallel()

	st := newFakeStore()

	llm := &fakeStreamingLLM{
		events: []provider.StreamEvent{
			{Type: provider.StreamEventTurnFinished},
		},
	}

	svc := NewService(st, llm, timeNowStub)

	result, err := svc.SendMessage(context.Background(), SendMessageParams{
		SessionID: "session-1",
		Prompt:    "hello",
	})
	require.NoError(t, err)
	require.Len(t, llm.calls, 1)
	require.Equal(t, QueryFinishReasonCompleted, result.FinishReason)
}

func TestServiceUsesMessageServiceInterface(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	svc := &Service{
		sessions: sessions,
		bus:      nil,
		nowFunc:  timeNowStub,
		query:    newStubQueryRunner(),
	}

	_, err := svc.SendMessage(context.Background(), SendMessageParams{
		SessionID: "session-1",
		Prompt:    "hello",
	})
	require.NoError(t, err)
}

func TestServiceSendMessageReturnsPendingToolCalls(t *testing.T) {
	t.Parallel()

	sessions := newStubSessionService()
	query := newStubQueryRunner()
	query.runResult = agent.QueryResult{
		SessionID:               "session-1",
		UserMessageID:           "user-1",
		FinalAssistantMessageID: "assistant-1",
		Turns:                   1,
		FinishReason:            agent.FinishReasonAwaitingToolExecution,
		PendingToolCalls: []provider.ToolCall{
			{
				Index:     0,
				ID:        "call_1",
				Name:      "search",
				Arguments: "{\"q\":\"golang\"}",
			},
		},
	}
	svc := newServiceWithDeps(sessions, &stubMessageService{}, query, timeNowStub)

	result, err := svc.SendMessage(context.Background(), SendMessageParams{
		SessionID: "session-1",
		Prompt:    "hello",
	})
	require.NoError(t, err)
	require.Equal(t, QueryFinishReasonAwaitingToolExecution, result.FinishReason)
	require.Len(t, result.PendingToolCalls, 1)
	require.Equal(t, "call_1", result.PendingToolCalls[0].ID)
}

func timeNowStub() time.Time {
	return time.Unix(1710004000, 0).UTC()
}

type fakeStore struct {
	sessions          []store.Session
	messagesBySession map[string][]store.Message
	createdMessages   []store.Message
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
	for _, item := range s.sessions {
		if item.ID == id {
			return item, nil
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

func (s *fakeStore) CreateMessage(_ context.Context, params store.CreateMessageParams) (store.Message, error) {
	msg := store.Message{
		ID:               params.ID,
		SessionID:        params.SessionID,
		Kind:             params.Kind,
		Provider:         params.Provider,
		FinishedAt:       params.FinishedAt,
		IsCompactSummary: params.IsCompactSummary,
		MessageJSON:      params.MessageJSON,
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
	calls  []provider.Request
}

func (f *fakeStreamingLLM) StreamChat(_ context.Context, req provider.Request) <-chan provider.StreamEvent {
	copied := provider.Request{}
	if len(req.Messages) > 0 {
		copied.Messages = append([]provider.Message(nil), req.Messages...)
	}
	f.calls = append(f.calls, copied)

	ch := make(chan provider.StreamEvent, len(f.events))
	for _, event := range f.events {
		ch <- event
	}
	close(ch)
	return ch
}

type stubMessageService struct {
	listResult        []message.Message
	lastListSessionID string
}

func (s *stubMessageService) Create(_ context.Context, _ string, _ message.CreateMessageParams) (message.Message, error) {
	return message.Message{}, nil
}

func (s *stubMessageService) Update(_ context.Context, _ message.Message) error {
	return nil
}

func (s *stubMessageService) Get(_ context.Context, _ string) (message.Message, error) {
	return message.Message{}, nil
}

func (s *stubMessageService) List(_ context.Context, sessionID string) ([]message.Message, error) {
	s.lastListSessionID = sessionID
	return append([]message.Message(nil), s.listResult...), nil
}

func (s *stubMessageService) ListUserMessages(_ context.Context, _ string) ([]message.Message, error) {
	return nil, nil
}

func (s *stubMessageService) ListAllUserMessages(_ context.Context) ([]message.Message, error) {
	return nil, nil
}

func (s *stubMessageService) Delete(_ context.Context, _ string) error {
	return nil
}

func (s *stubMessageService) DeleteSessionMessages(_ context.Context, _ string) error {
	return nil
}

func (s *stubMessageService) Events() <-chan message.Event {
	return make(chan message.Event)
}

type stubQueryRunner struct {
	bus       bus.Bus[agent.Event]
	events    <-chan agent.Event
	runResult agent.QueryResult
}

func newStubQueryRunner() *stubQueryRunner {
	b := bus.NewBus[agent.Event](8)
	return &stubQueryRunner{
		bus:    b,
		events: b.Subscribe(context.Background()),
		runResult: agent.QueryResult{
			SessionID:               "session-1",
			UserMessageID:           "user-1",
			FinalAssistantMessageID: "assistant-1",
			Turns:                   1,
			FinishReason:            agent.FinishReasonCompleted,
		},
	}
}

func (s *stubQueryRunner) RunQuery(_ context.Context, _ agent.QueryParams) (agent.QueryResult, error) {
	return s.runResult, nil
}

func (s *stubQueryRunner) Events() <-chan agent.Event {
	return s.events
}

func (s *stubQueryRunner) publish(event agent.Event) {
	s.bus.Publish(event)
}

func newServiceWithDeps(sessionSvc sessionService, msgSvc messageService, query queryRunner, nowFunc func() time.Time) *Service {
	if nowFunc == nil {
		nowFunc = time.Now
	}

	appBus := bus.NewBus[Event](128)
	svc := &Service{
		sessions: sessionSvc,
		bus:      appBus,
		events:   appBus.Subscribe(context.Background()),
		nowFunc:  nowFunc,
		messages: msgSvc,
		query:    query,
	}

	go svc.forwardSessionEvents(sessionSvc.Events())
	go svc.forwardMessageEvents(msgSvc.Events())
	go svc.forwardAgentEvents(query.Events())

	return svc
}

func messageRecord(id string, kind message.Kind, text string) message.Message {
	return message.Message{
		ID:     id,
		Kind:   kind,
		Status: message.StatusComplete,
		Parts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: text,
			},
		},
	}
}

type stubSessionService struct {
	bus           bus.Bus[session.Event]
	events        <-chan session.Event
	listResult    []store.Session
	createdTitles []string
}

func newStubSessionService() *stubSessionService {
	b := bus.NewBus[session.Event](8)
	return &stubSessionService{
		bus:    b,
		events: b.Subscribe(context.Background()),
	}
}

func (s *stubSessionService) Create(_ context.Context, title string) (store.Session, error) {
	s.createdTitles = append(s.createdTitles, title)

	created := store.Session{
		ID:        "session-created",
		Title:     title,
		CreatedAt: timeNowStub(),
		UpdatedAt: timeNowStub(),
	}
	s.listResult = append([]store.Session{created}, s.listResult...)
	return created, nil
}

func (s *stubSessionService) Get(_ context.Context, id string) (store.Session, error) {
	for _, item := range s.listResult {
		if item.ID == id {
			return item, nil
		}
	}
	return store.Session{}, session.ErrSessionNotFound
}

func (s *stubSessionService) GetLast(_ context.Context) (store.Session, error) {
	if len(s.listResult) == 0 {
		return store.Session{}, session.ErrSessionNotFound
	}
	return s.listResult[0], nil
}

func (s *stubSessionService) List(_ context.Context) ([]store.Session, error) {
	return append([]store.Session(nil), s.listResult...), nil
}

func (s *stubSessionService) Rename(_ context.Context, id string, title string) (store.Session, error) {
	for i := range s.listResult {
		if s.listResult[i].ID != id {
			continue
		}
		s.listResult[i].Title = title
		return s.listResult[i], nil
	}
	return store.Session{}, session.ErrSessionNotFound
}

func (s *stubSessionService) Delete(_ context.Context, id string) error {
	for i := range s.listResult {
		if s.listResult[i].ID != id {
			continue
		}
		s.listResult = append(s.listResult[:i], s.listResult[i+1:]...)
		return nil
	}
	return session.ErrSessionNotFound
}

func (s *stubSessionService) Events() <-chan session.Event {
	return s.events
}
