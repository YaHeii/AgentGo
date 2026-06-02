package app

type EventType string

const (
	EventBootstrap EventType = "lifecycle"
	EventMessage   EventType = "message"
	EventSession   EventType = "session"
	EventAgent     EventType = "agent"
	EventProvider  EventType = "provider"
)

type Event interface {
	Type() EventType
	Data() any
}

type BaseEvent struct {
	T       EventType
	Payload any
}

func (e BaseEvent) Type() EventType { return e.T }
func (e BaseEvent) Data() any       { return e.Payload }
