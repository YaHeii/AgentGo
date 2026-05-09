package session

import "github.com/YaHeii/agentGo/internal/store"

type Event interface {
	isSessionEvent()
}

type SessionCreatedEvent struct {
	Session store.Session
}

func (SessionCreatedEvent) isSessionEvent() {}

type SessionUpdatedEvent struct {
	Session store.Session
}

func (SessionUpdatedEvent) isSessionEvent() {}

type SessionDeletedEvent struct {
	SessionID string
}

func (SessionDeletedEvent) isSessionEvent() {}
