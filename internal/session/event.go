package session

import (
	"fmt"

	"github.com/YaHeii/agentGo/internal/app"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
)

type SessionStatus int

const (
	StatusCreated  SessionStatus = iota // 0
	StatusUpdated                       // 1
	StatusDeleted                       // 2
	StatusSwitched                      // 3
	StatusRestored                      // 4
)

func (s SessionStatus) String() string {
	switch s {
	case StatusCreated:
		return "CREATED"
	case StatusUpdated:
		return "UPDATED"
	case StatusDeleted:
		return "DELETED"
	case StatusSwitched:
		return "SWITCHED"
	case StatusRestored:
		return "RESTORED"
	default:
		return fmt.Sprintf("UNKNOWN_STATUS(%d)", s)
	}
}

type SessionEvent struct {
	Status  SessionStatus
	Session *sessioncontract.Session
}

func NewSessionCreatedEvent(session sessioncontract.Session) app.Event {
	return newSessionEvent(app.EventSessionCreated, StatusCreated, &session)
}

func NewSessionUpdatedEvent(session sessioncontract.Session) app.Event {
	return newSessionEvent(app.EventSessionUpdated, StatusUpdated, &session)
}

func NewSessionDeletedEvent() app.Event {
	return newSessionEvent(app.EventSessionDeleted, StatusDeleted, nil)
}

func NewSessionSwitchedEvent(session sessioncontract.Session) app.Event {
	return newSessionEvent(app.EventSessionSwitched, StatusSwitched, &session)
}

func NewSessionRestoredEvent(session sessioncontract.Session) app.Event {
	return newSessionEvent(app.EventSessionRestored, StatusRestored, &session)
}

func newSessionEvent(name app.EventName, status SessionStatus, session *sessioncontract.Session) app.Event {
	return app.NewEvent(name, SessionEvent{
		Status:  status,
		Session: session,
	})
}
