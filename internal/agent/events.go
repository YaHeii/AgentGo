package agent

import "github.com/YaHeii/agentGo/internal/app"

type Event interface {
	app.Event
	isAgentEvent()
}

type QueryCompletedEvent struct {
	SessionID          string
	UserMessageID      string
	AssistantMessageID string
}

func (QueryCompletedEvent) isAgentEvent() {}

func (QueryCompletedEvent) Type() app.EventType { return app.EventTypeAgent }

type QueryFailedEvent struct {
	SessionID          string
	UserMessageID      string
	AssistantMessageID string
	Err                error
}

func (QueryFailedEvent) isAgentEvent() {}

func (QueryFailedEvent) Type() app.EventType { return app.EventTypeAgent }
