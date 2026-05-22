package session

import (
	"context"
	"errors"
	"time"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
	"github.com/segmentio/ksuid"
)

var ErrSessionNotFound = sessioncontract.ErrSessionNotFound

type SessionService struct {
	sessionStore    sessionStore
	messageStore    messageStore
	dispatcher      app.Dispatcher
	nowFunc         func() time.Time
	activeSessionID string
	parentSessionID string
}

type RestoreResult struct {
	Session  sessioncontract.Session
	Messages []messagecontract.Message
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

func (s *SessionService) Create(ctx context.Context, title string, d app.Dispatcher) (string, error) {
	now := s.nowFunc().UTC()
	sessionID, err := ksuid.NewRandomWithTime(now)
	if err != nil {
		return "", err
	}
	row, err := s.sessionStore.CreateSession(ctx, sessioncontract.CreateSessionParams{
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
			Session: sessionPtr(row),
		},
	})
	return row.ID, nil
}

func (s *SessionService) Get(ctx context.Context, id string) (sessioncontract.Session, error) {
	row, err := s.sessionStore.GetSession(ctx, id)
	if err != nil {
		return sessioncontract.Session{}, mapError(err)
	}

	return row, nil
}

func (s *SessionService) Save(ctx context.Context, session sessioncontract.Session) (sessioncontract.Session, error) {
	updatedAt := s.nowFunc().UTC()
	row, err := s.sessionStore.UpdateSession(ctx, sessioncontract.UpdateSessionParams{
		ID:               session.ID,
		ParentSessionID:  session.ParentSessionID,
		Title:            session.Title,
		MessageCount:     session.MessageCount,
		CompletionTokens: session.CompletionTokens,
		CostMicros:       session.CostMicros,
		SummaryMessageID: session.SummaryMessageID,
		TodosJSON:        session.TodosJSON,
		UpdatedAt:        updatedAt,
	})
	if err != nil {
		return sessioncontract.Session{}, mapError(err)
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

func (s *SessionService) List(ctx context.Context) ([]sessioncontract.Session, error) {
	rows, err := s.sessionStore.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	return rows, nil
}

func (s *SessionService) Rename(ctx context.Context, id string, title string, d app.Dispatcher) (sessioncontract.Session, error) {
	current, err := s.sessionStore.GetSession(ctx, id)
	if err != nil {
		return sessioncontract.Session{}, mapError(err)
	}

	updatedAt := s.nowFunc().UTC()
	row, err := s.sessionStore.UpdateSession(ctx, sessioncontract.UpdateSessionParams{
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
		return sessioncontract.Session{}, mapError(err)
	}

	session := row
	d.Dispatch(app.BaseEvent{
		T: "session",
		Payload: SessionEvent{
			Status:  StatusUpdated,
			Session: sessionPtr(session),
		},
	})
	return session, nil
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
	session := sessionRow
	d.Dispatch(app.BaseEvent{
		T: "session",
		Payload: SessionEvent{
			Status:  StatusUpdated,
			Session: sessionPtr(session),
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

func (s *SessionService) CreateMessage(ctx context.Context, params messagecontract.CreateMessageParams, d app.Dispatcher) (messagecontract.Message, error) {
	msg, err := s.messageStore.CreateMessage(ctx, params)
	if err != nil {
		return messagecontract.Message{}, err
	}

	d.Dispatch(app.BaseEvent{
		T: app.EventMessage,
		Payload: message.MessageEvent{
			Status:  message.StatusPending,
			Message: &msg,
		},
	})
	return msg, nil
}

func (s *SessionService) ListHistory(ctx context.Context, sessionID string, d app.Dispatcher) ([]messagecontract.Message, error) {
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
			Session: sessionPtr(result.Session),
		},
	})
	return result, nil
}

func (s *SessionService) listMessages(ctx context.Context, sessionID string, d app.Dispatcher) ([]messagecontract.Message, error) {
	return s.messageStore.ListMessages(ctx, sessionID)
}

func mapError(err error) error {
	if errors.Is(err, sessioncontract.ErrSessionNotFound) {
		return ErrSessionNotFound
	}

	return err
}

func sessionPtr(session sessioncontract.Session) *sessioncontract.Session {
	return &session
}
