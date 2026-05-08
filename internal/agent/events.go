package agent

type Event interface {
	isAgentEvent()
}

type QueryCompletedEvent struct {
	SessionID          string
	UserMessageID      string
	AssistantMessageID string
}

func (QueryCompletedEvent) isAgentEvent() {}

type QueryFailedEvent struct {
	SessionID          string
	UserMessageID      string
	AssistantMessageID string
	Err                error
}

func (QueryFailedEvent) isAgentEvent() {}
