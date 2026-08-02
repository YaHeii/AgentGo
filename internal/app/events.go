package app

type EventType string
type EventName string

const (
	EventBootstrap EventType = "lifecycle"
	EventMessage   EventType = "message"
	EventSession   EventType = "session"
	EventAgent     EventType = "agent"
	EventProvider  EventType = "provider"
)

const (
	EventSessionCreated  EventName = "session.created"
	EventSessionUpdated  EventName = "session.updated"
	EventSessionDeleted  EventName = "session.deleted"
	EventSessionSwitched EventName = "session.switched"
	EventSessionRestored EventName = "session.restored"

	EventMessageCreated EventName = "message.created"

	EventAgentLoopStarted      EventName = "agent.loop.started"
	EventAgentLoopAwaitingTool EventName = "agent.loop.awaiting_tool"
	EventAgentLoopCompleted    EventName = "agent.loop.completed"
	EventAgentLoopFailed       EventName = "agent.loop.failed"
	EventAgentLoopCancelled    EventName = "agent.loop.cancelled"

	EventProviderTextDelta      EventName = "provider.text_delta"
	EventProviderReasoningDelta EventName = "provider.reasoning_delta"
	EventProviderRefusalDelta   EventName = "provider.refusal_delta"
	EventProviderToolCallDelta  EventName = "provider.tool_call_delta"
	EventProviderUsageAvailable EventName = "provider.usage_available"
	EventProviderTurnFinished   EventName = "provider.turn_finished"
	EventProviderError          EventName = "provider.error"
)

type Event interface {
	Type() EventType
	Name() EventName
	Data() any
}

type BaseEvent struct {
	T       EventType
	N       EventName
	Payload any
}

func (e BaseEvent) Type() EventType { return e.T }
func (e BaseEvent) Name() EventName { return e.N }
func (e BaseEvent) Data() any       { return e.Payload }
