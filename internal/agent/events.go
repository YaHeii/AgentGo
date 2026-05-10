package agent

type QueryCompletedEvent struct {
	SessionID          string
	UserMessageID      string
	AssistantMessageID string
}

type QueryFailedEvent struct {
	SessionID          string
	UserMessageID      string
	AssistantMessageID string
	Err                error
}
