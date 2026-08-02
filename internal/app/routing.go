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
	EventSessionCreated: {
		Type: EventSession,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventSessionCreated),
		},
	},
	EventSessionUpdated: {
		Type: EventSession,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventSessionUpdated),
		},
	},
	EventSessionDeleted: {
		Type: EventSession,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventSessionDeleted),
		},
	},
	EventSessionSwitched: {
		Type: EventSession,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventSessionSwitched),
		},
	},
	EventSessionRestored: {
		Type: EventSession,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventSessionRestored),
		},
	},
	EventMessageCreated: {
		Type: EventMessage,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventMessageCreated),
		},
	},
	EventAgentLoopStarted: {
		Type: EventAgent,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventAgentLoopStarted),
		},
	},
	EventAgentLoopAwaitingTool: {
		Type: EventAgent,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventAgentLoopAwaitingTool),
		},
	},
	EventAgentLoopCompleted: {
		Type: EventAgent,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventAgentLoopCompleted),
		},
	},
	EventAgentLoopFailed: {
		Type: EventAgent,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventAgentLoopFailed),
		},
	},
	EventAgentLoopCancelled: {
		Type: EventAgent,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventAgentLoopCancelled),
		},
	},
	EventProviderTextDelta: {
		Type: EventProvider,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventProviderTextDelta),
		},
	},
	EventProviderReasoningDelta: {
		Type: EventProvider,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventProviderReasoningDelta),
		},
	},
	EventProviderRefusalDelta: {
		Type: EventProvider,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventProviderRefusalDelta),
		},
	},
	EventProviderToolCallDelta: {
		Type: EventProvider,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventProviderToolCallDelta),
		},
	},
	EventProviderUsageAvailable: {
		Type: EventProvider,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventProviderUsageAvailable),
		},
	},
	EventProviderTurnFinished: {
		Type: EventProvider,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventProviderTurnFinished),
		},
	},
	EventProviderError: {
		Type: EventProvider,
		Route: RoutePolicy{
			Delivery:   DeliveryMemory,
			RoutingKey: string(EventProviderError),
		},
	},
}

func routeForEvent(evt Event) (RoutePolicy, bool) {
	if evt == nil {
		return RoutePolicy{}, false
	}
	if name := evt.Name(); name != "" {
		meta, ok := eventCatalog[name]
		return meta.Route, ok
	}
	return RoutePolicy{}, false
}
