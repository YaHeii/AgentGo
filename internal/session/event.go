package session

import (
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/store"
)

type Event interface {
	app.Event
	isSessionEvent()
}

type SessionCreatedEvent struct {
	Session store.Session
}

func (SessionCreatedEvent) isSessionEvent() {}

func (SessionCreatedEvent) Type() app.EventType { return app.EventTypeSession }

type SessionUpdatedEvent struct {
	Session store.Session
}

func (SessionUpdatedEvent) isSessionEvent() {}

func (SessionUpdatedEvent) Type() app.EventType { return app.EventTypeSession }

type SessionDeletedEvent struct {
	SessionID string
}

func (SessionDeletedEvent) isSessionEvent() {}

func (SessionDeletedEvent) Type() app.EventType { return app.EventTypeSession }

type SessionSwitchedEvent struct {
	SessionID string
}

func (SessionSwitchedEvent) isSessionEvent() {}

func (SessionSwitchedEvent) Type() app.EventType { return app.EventTypeSession }

type SessionRestoredEvent struct {
	Session  store.Session
	Messages []message.Message
}

func (SessionRestoredEvent) isSessionEvent() {}

func (SessionRestoredEvent) Type() app.EventType { return app.EventTypeSession }
