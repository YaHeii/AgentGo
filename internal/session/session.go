package session

import "time"

// Session describes the persisted metadata of a conversation container.
type Session struct {
	ID           string
	Title        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastActiveAt time.Time

	// TODO: Add ParentSessionID when parent-child session topology is implemented.
	// TODO: Add SummaryMessageID when compact summary persistence is introduced.
	// TODO: Add PromptTokens, CompletionTokens, and CostMicros when usage aggregation is implemented.
}
