package app

type EventType string

const (
	EventTypeSession   EventType = "session"
	EventTypeMessage   EventType = "message"
	EventTypeAgent     EventType = "agent"
	EventTypeLifecycle EventType = "lifecycle"
)

type Event interface {
	Type() EventType
}
