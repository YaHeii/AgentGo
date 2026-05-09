package session

import (
	"context"
	"errors"
	"time"

	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/store"
)

var ErrSessionNotFound = errors.New("session: not found")
// NOTE: The session business logic is not complex enough at present,
//  so only a simple wrapper is applied to the store layer.
// TODO: The next implementation is the token calculation.
// use event to notify the UI layer
// define interface to manage messages
type SessionService struct {
	store   sessionStore
	bus     bus.Bus[Event]
	events  <-chan Event
	nowFunc func() time.Time
}

type sessionStore interface {
	CreateSession(ctx context.Context, params store.CreateSessionParams) (store.Session, error)
	ListSessions(ctx context.Context) ([]store.Session, error)
	GetSession(ctx context.Context, id string) (store.Session, error)
	UpdateSession(ctx context.Context, params store.UpdateSessionParams) (store.Session, error)
	DeleteSession(ctx context.Context, id string) error
}

func NewSessionService(st sessionStore, nowFunc func() time.Time) *SessionService {
	if nowFunc == nil {
		nowFunc = time.Now
	}

	b := bus.NewBus[Event](64)
	return &SessionService{
		store:   st,
		bus:     b,
		events:  b.Subscribe(context.Background()),
		nowFunc: nowFunc,
	}
}

func (s *SessionService) Create(ctx context.Context, title string) (store.Session, error) {
	now := s.nowFunc().UTC()

	row, err := s.store.CreateSession(ctx, store.CreateSessionParams{
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
	row, err := s.store.GetSession(ctx, id)
	if err != nil {
		return store.Session{}, mapError(err)
	}

	return row, nil
}

func (s *SessionService) GetLast(ctx context.Context) (store.Session, error) {
	rows, err := s.store.ListSessions(ctx)
	if err != nil {
		return store.Session{}, err
	}
	if len(rows) == 0 {
		return store.Session{}, ErrSessionNotFound
	}

	return rows[0], nil
}

func (s *SessionService) List(ctx context.Context) ([]store.Session, error) {
	rows, err := s.store.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	return rows, nil
}

func (s *SessionService) Rename(ctx context.Context, id string, title string) (store.Session, error) {
	current, err := s.store.GetSession(ctx, id)
	if err != nil {
		return store.Session{}, mapError(err)
	}

	updatedAt := s.nowFunc().UTC()
	row, err := s.store.UpdateSession(ctx, store.UpdateSessionParams{
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
	if err := s.store.DeleteSession(ctx, id); err != nil {
		return mapError(err)
	}

	s.publish(SessionDeletedEvent{SessionID: id})
	return nil
}

func (s *SessionService) Events() <-chan Event {
	return s.events
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
