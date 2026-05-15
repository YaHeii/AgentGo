package session

import (
	"fmt"

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
