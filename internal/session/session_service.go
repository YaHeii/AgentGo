package session

import (
	"context"
	"errors"
	"time"

	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/store"
)

var ErrSessionNotFound = store.ErrSessionNotFound

type SessionService struct {
	sessionStore    sessionStore
	messageStore    messageStore
	bus             bus.Bus[Event]
	events          <-chan Event
	nowFunc         func() time.Time
	activeSessionID string
	parentSessionID string
}
type RestoreResult struct {
	Session  store.Session
	Messages []message.Message
}

func NewSessionService(st sessionStore, msgs messageStore, nowFunc func() time.Time) *SessionService {
	if nowFunc == nil {
		nowFunc = time.Now
	}

	b := bus.NewBus[Event](64)
	return &SessionService{
		sessionStore:    st,
		messageStore:    msgs,
		bus:             b,
		events:          b.Subscribe(context.Background()),
		nowFunc:         nowFunc,
		activeSessionID: "",
		parentSessionID: "",
	}
}

func (s *SessionService) Create(ctx context.Context, title string) (store.Session, error) {
	now := s.nowFunc().UTC()

	row, err := s.sessionStore.CreateSession(ctx, store.CreateSessionParams{
		ID:        "session-" + now.Format(time.RFC3339Nano),
		Title:     title,
		TodosJSON: "[]",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return store.Session{}, err
	}

	s.publish(SessionCreatedEvent{Session: row})
	return row, nil
}

func (s *SessionService) Get(ctx context.Context, id string) (store.Session, error) {
	row, err := s.sessionStore.GetSession(ctx, id)
	if err != nil {
		return store.Session{}, mapError(err)
	}

	return row, nil
}

func (s *SessionService) GetLast(ctx context.Context) (store.Session, error) {
	rows, err := s.sessionStore.ListSessions(ctx)
	if err != nil {
		return store.Session{}, err
	}
	if len(rows) == 0 {
		return store.Session{}, ErrSessionNotFound
	}

	return rows[0], nil
}

func (s *SessionService) List(ctx context.Context) ([]store.Session, error) {
	rows, err := s.sessionStore.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	return rows, nil
}

func (s *SessionService) Rename(ctx context.Context, id string, title string) (store.Session, error) {
	current, err := s.sessionStore.GetSession(ctx, id)
	if err != nil {
		return store.Session{}, mapError(err)
	}

	updatedAt := s.nowFunc().UTC()
	row, err := s.sessionStore.UpdateSession(ctx, store.UpdateSessionParams{
		ID:               id,
		ParentSessionID:  current.ParentSessionID,
		Title:            title,
		MessageCount:     current.MessageCount,
		CompletionTokens: current.CompletionTokens,
		CostMicros:       current.CostMicros,
		SummaryMessageID: current.SummaryMessageID,
		TodosJSON:        current.TodosJSON,
		UpdatedAt:        updatedAt,
	})
	if err != nil {
		return store.Session{}, mapError(err)
	}

	s.publish(SessionUpdatedEvent{Session: row})
	return row, nil
}

func (s *SessionService) Delete(ctx context.Context, id string) error {
	if err := s.sessionStore.DeleteSession(ctx, id); err != nil {
		return mapError(err)
	}

	s.publish(SessionDeletedEvent{SessionID: id})
	return nil
}

func (s *SessionService) Events() <-chan Event {
	return s.events
}

func (s *SessionService) GetSessionID() string {
	return s.activeSessionID
}

func (s *SessionService) RegenerateSessionID() string {
	now := s.nowFunc().UTC()
	s.activeSessionID = "session-" + now.Format(time.RFC3339Nano)
	return s.activeSessionID
}

func (s *SessionService) GetParentSessionID() string {
	return s.parentSessionID
}

func (s *SessionService) SwitchSession(ctx context.Context, sessionID string) error {
	sessionRow, err := s.sessionStore.GetSession(ctx, sessionID)
	if err != nil {
		return mapError(err)
	}

	s.activeSessionID = sessionID
	s.parentSessionID = sessionRow.ParentSessionID
	s.publish(SessionSwitchedEvent{SessionID: sessionID})
	return nil
}

func (s *SessionService) CreateMessage(ctx context.Context, sessionID string, params message.CreateMessageParams) (message.Message, error) {
	return s.messageStore.CreateMessage(ctx, sessionID, params)
}

func (s *SessionService) ListHistory(ctx context.Context, sessionID string) ([]message.Message, error) {
	return s.listMessages(ctx, sessionID)
}

func (s *SessionService) UpdateMessage(ctx context.Context, sessionID string, msg message.Message) error {
	return s.messageStore.UpdateMessage(ctx, sessionID, msg)
}

func (s *SessionService) RestoreSession(ctx context.Context, sessionID string) (RestoreResult, error) {
	return s.restoreSession(ctx, sessionID)
}

func (s *SessionService) Restore(ctx context.Context, sessionID string) error {
	_, err := s.restoreSession(ctx, sessionID)
	return err
}

func (s *SessionService) restoreSession(ctx context.Context, sessionID string) (RestoreResult, error) {
	sessionRow, err := s.sessionStore.GetSession(ctx, sessionID)
	if err != nil {
		return RestoreResult{}, mapError(err)
	}

	msgs, err := s.listMessages(ctx, sessionID)
	if err != nil {
		return RestoreResult{}, err
	}

	result := RestoreResult{
		Session:  sessionRow,
		Messages: msgs,
	}
	s.activeSessionID = sessionID
	s.parentSessionID = sessionRow.ParentSessionID
	s.publish(SessionRestoredEvent{
		Session:  sessionRow,
		Messages: msgs,
	})

	return result, nil
}

func (s *SessionService) listMessages(ctx context.Context, sessionID string) ([]message.Message, error) {
	return s.messageStore.ListMessages(ctx, sessionID)
}

func (s *SessionService) publish(event Event) {
	if s.bus == nil {
		return
	}

	s.bus.Publish(event)
}

func mapError(err error) error {
	if errors.Is(err, store.ErrSessionNotFound) {
		return ErrSessionNotFound
	}

	return err
}
