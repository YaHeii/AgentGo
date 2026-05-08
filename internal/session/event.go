package session

type Event interface {
	isSessionEvent()
}

type SessionCreatedEvent struct {
	Session Session
}

func (SessionCreatedEvent) isSessionEvent() {}

type SessionUpdatedEvent struct {
	Session Session
}

func (SessionUpdatedEvent) isSessionEvent() {}

type SessionDeletedEvent struct {
	SessionID string
}

func (SessionDeletedEvent) isSessionEvent() {}
