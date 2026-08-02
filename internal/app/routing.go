package app

type Delivery uint8

const (
	DeliveryMemory Delivery = 1 << iota
	DeliveryMQ
)

type RoutePolicy struct {
	Delivery   Delivery
	RoutingKey string
}

type eventMeta struct {
	Type  EventType
	Route RoutePolicy
}

func NewEvent(name EventName, payload any) Event {
	meta := eventCatalog[name]
	return BaseEvent{
		T:       meta.Type,
		N:       name,
		Payload: payload,
	}
}

var eventCatalog = map[EventName]eventMeta{
	EventSessionCreated:  memoryEvent(EventSessionCreated, EventSession),
	EventSessionUpdated:  memoryEvent(EventSessionUpdated, EventSession),
	EventSessionDeleted:  memoryEvent(EventSessionDeleted, EventSession),
	EventSessionSwitched: memoryEvent(EventSessionSwitched, EventSession),
	EventSessionRestored: memoryEvent(EventSessionRestored, EventSession),

	EventMessageCreated: memoryEvent(EventMessageCreated, EventMessage),

	EventAgentLoopStarted:      memoryEvent(EventAgentLoopStarted, EventAgent),
	EventAgentLoopAwaitingTool: memoryEvent(EventAgentLoopAwaitingTool, EventAgent),
	EventAgentLoopCompleted:    memoryEvent(EventAgentLoopCompleted, EventAgent),
	EventAgentLoopFailed:       memoryEvent(EventAgentLoopFailed, EventAgent),
	EventAgentLoopCancelled:    memoryEvent(EventAgentLoopCancelled, EventAgent),

	EventProviderTextDelta:      memoryEvent(EventProviderTextDelta, EventProvider),
	EventProviderReasoningDelta: memoryEvent(EventProviderReasoningDelta, EventProvider),
	EventProviderRefusalDelta:   memoryEvent(EventProviderRefusalDelta, EventProvider),
	EventProviderToolCallDelta:  memoryEvent(EventProviderToolCallDelta, EventProvider),
	EventProviderUsageAvailable: memoryEvent(EventProviderUsageAvailable, EventProvider),
	EventProviderTurnFinished:   memoryEvent(EventProviderTurnFinished, EventProvider),
	EventProviderError:          memoryEvent(EventProviderError, EventProvider),
}

func memoryEvent(name EventName, eventType EventType) eventMeta {
	return eventMeta{
		Type: eventType,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(name),
		},
	}
}

func routeForEvent(evt Event) (RoutePolicy, bool) {
	if evt == nil {
		return RoutePolicy{}, false
	}
	if name := evt.Name(); name != "" {
		meta, ok := eventCatalog[name]
		return meta.Route, ok
	}
	if eventType := evt.Type(); eventType != "" {
		return RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(eventType),
		}, true
	}
	return RoutePolicy{}, false
}
