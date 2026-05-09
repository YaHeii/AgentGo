package session

import (
	"time"
)

// Session describes the persisted metadata of a conversation container.
type Session struct {
	ID               string
	ParentSessionID  string
	Title            string
	MessageCount     int64
	CompletionTokens int64
	CostMicros       int64
	SummaryMessageID string
	TodosJSON        string
	CreatedAt        time.Time
	UpdatedAt        time.Time

	// TODO: Add ParentSessionID when parent-child session topology is implemented.
	// TODO: Add SummaryMessageID when compact summary persistence is introduced.
	// TODO: Refine todos structure once agent-side todo orchestration is implemented.
}
