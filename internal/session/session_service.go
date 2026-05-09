package session

import (
	"context"
	"errors"
	"time"

	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/store"
)

var ErrSessionNotFound = errors.New("session: not found")

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

func (s *SessionService) Create(ctx context.Context, title string) (Session, error) {
	now := s.nowFunc().UTC()

	row, err := s.store.CreateSession(ctx, store.CreateSessionParams{
		ID:        "session-" + now.Format(time.RFC3339Nano),
		Title:     title,
		TodosJSON: "[]",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return Session{}, err
	}

	session := toSession(row)
	s.publish(SessionCreatedEvent{Session: session})
	return session, nil
}

func (s *SessionService) Get(ctx context.Context, id string) (Session, error) {
	row, err := s.store.GetSession(ctx, id)
	if err != nil {
		return Session{}, mapError(err)
	}

	return toSession(row), nil
}

func (s *SessionService) GetLast(ctx context.Context) (Session, error) {
	rows, err := s.store.ListSessions(ctx)
	if err != nil {
		return Session{}, err
	}
	if len(rows) == 0 {
		return Session{}, ErrSessionNotFound
	}

	return toSession(rows[0]), nil
}

func (s *SessionService) List(ctx context.Context) ([]Session, error) {
	rows, err := s.store.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]Session, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSession(row))
	}

	return out, nil
}

func (s *SessionService) Rename(ctx context.Context, id string, title string) (Session, error) {
	current, err := s.store.GetSession(ctx, id)
	if err != nil {
		return Session{}, mapError(err)
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
		return Session{}, mapError(err)
	}

	session := toSession(row)
	s.publish(SessionUpdatedEvent{Session: session})
	return session, nil
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

func toSession(row store.Session) Session {
	return Session{
		ID:               row.ID,
		ParentSessionID:  row.ParentSessionID,
		Title:            row.Title,
		MessageCount:     row.MessageCount,
		CompletionTokens: row.CompletionTokens,
		CostMicros:       row.CostMicros,
		SummaryMessageID: row.SummaryMessageID,
		TodosJSON:        row.TodosJSON,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func mapError(err error) error {
	if errors.Is(err, store.ErrSessionNotFound) {
		return ErrSessionNotFound
	}

	return err
}
