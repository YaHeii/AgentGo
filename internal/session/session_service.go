package session

import (
	"context"
	"errors"
	"time"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/segmentio/ksuid"
)

var ErrSessionNotFound = store.ErrSessionNotFound

type SessionService struct {
	sessionStore    sessionStore
	messageStore    messageStore
	dispatcher      app.Dispatcher
	nowFunc         func() time.Time
	activeSessionID string
	parentSessionID string
}
type RestoreResult struct {
	Session  store.Session
	Messages []message.Message
}

func NewSessionService(st sessionStore, msgs messageStore, d app.Dispatcher) *SessionService {

	return &SessionService{
		sessionStore:    st,
		messageStore:    msgs,
		dispatcher:      d,
		nowFunc:         time.Now,
		activeSessionID: "",
		parentSessionID: "",
	}
}

func (s *SessionService) Create(ctx context.Context, title string, d app.Dispatcher) (string ,error) {
	now := s.nowFunc().UTC()
	sessionID, err := ksuid.NewRandomWithTime(now)
	if err != nil {
		return "", err
	}
	row, err := s.sessionStore.CreateSession(ctx, store.CreateSessionParams{
		ID:        sessionID.String(),
		Title:     title,
		TodosJSON: "[]",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return "", err
	}

	d.Dispatch(app.BaseEvent{
		T: "session",
		Payload: SessionEvent{
			Status:  StatusCreated,
			Session: &row,
		},
	})
	return row.ID, nil
}

func (s *SessionService) Get(ctx context.Context, id string) (store.Session, error) {
	row, err := s.sessionStore.GetSession(ctx, id)
	if err != nil {
		return store.Session{}, mapError(err)
	}

	return row, nil
}

func (s *SessionService) GetLast(ctx context.Context) (string, error) {
	rows, err := s.sessionStore.ListSessions(ctx)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", ErrSessionNotFound
	}

	return rows[0].ID, nil
}

func (s *SessionService) List(ctx context.Context) ([]store.Session, error) {
	rows, err := s.sessionStore.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	return rows, nil
}

func (s *SessionService) Rename(ctx context.Context, id string, title string, d app.Dispatcher) (store.Session, error) {
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
	d.Dispatch(app.BaseEvent{
		T: "session",
		Payload: SessionEvent{
			Status:  StatusUpdated,
			Session: &row,
		},
	})
	return row, nil
}

func (s *SessionService) Delete(ctx context.Context, id string, d app.Dispatcher) error {
	if err := s.sessionStore.DeleteSession(ctx, id); err != nil {
		return mapError(err)
	}

	d.Dispatch(app.BaseEvent{
		T: "session",
		Payload: SessionEvent{
			Status:  StatusUpdated,
			Session: nil,
		},
	})
	return nil
}

func (s *SessionService) SwitchSession(ctx context.Context, sessionID string, d app.Dispatcher) error {
	sessionRow, err := s.sessionStore.GetSession(ctx, sessionID)
	if err != nil {
		return mapError(err)
	}

	s.activeSessionID = sessionID
	s.parentSessionID = sessionRow.ParentSessionID
	d.Dispatch(app.BaseEvent{
		T: "session",
		Payload: SessionEvent{
			Status:  StatusUpdated,
			Session: &sessionRow,
		},
	})
	return nil
}

func (s *SessionService) GetSessionID() string {
	return s.activeSessionID
}

func (s *SessionService) GetParentSessionID() string {
	return s.parentSessionID
}

func (s *SessionService) CreateMessage(ctx context.Context, sessionID string, params message.CreateMessageParams, d app.Dispatcher) (message.Message, error) {
	return s.messageStore.CreateMessage(ctx, sessionID, params, d)
}

func (s *SessionService) ListHistory(ctx context.Context, sessionID string, d app.Dispatcher) ([]message.Message, error) {
	return s.listMessages(ctx, sessionID, d)
}

func (s *SessionService) Restore(ctx context.Context, sessionID string, d app.Dispatcher) error {
	_, err := s.restoreSession(ctx, sessionID, d)
	return err
}

func (s *SessionService) restoreSession(ctx context.Context, sessionID string, d app.Dispatcher) (RestoreResult, error) {
	sessionRow, err := s.sessionStore.GetSession(ctx, sessionID)
	if err != nil {
		return RestoreResult{}, mapError(err)
	}

	msgs, err := s.listMessages(ctx, sessionID, d)
	if err != nil {
		return RestoreResult{}, err
	}

	result := RestoreResult{
		Session:  sessionRow,
		Messages: msgs,
	}
	s.activeSessionID = sessionID
	s.parentSessionID = sessionRow.ParentSessionID

	d.Dispatch(app.BaseEvent{
		T: "session",
		Payload: SessionEvent{
			Status:  StatusRestored,
			Session: &sessionRow,
		},
	})
	return result, nil
}

func (s *SessionService) listMessages(ctx context.Context, sessionID string, d app.Dispatcher) ([]message.Message, error) {
	return s.messageStore.ListMessages(ctx, sessionID, d)
}

func mapError(err error) error {
	if errors.Is(err, store.ErrSessionNotFound) {
		return ErrSessionNotFound
	}

	return err
}
