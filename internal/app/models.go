package app

import "time"

type MessageStatus string

const (
	MessageStatusComplete  MessageStatus = "complete"
	MessageStatusStreaming MessageStatus = "streaming"
	MessageStatusCancelled MessageStatus = "cancelled"
	MessageStatusFailed    MessageStatus = "failed"
)

// TODO: Abstract session layer
type Session struct {
	ID           string
	Title        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastActiveAt time.Time

	// TODO: Add ParentSessionID when session topology is introduced.
	// TODO: Add SummaryMessageID and usage fields when session metadata expands.
}

type Message struct {
	ID        string
	SessionID string
	Role      string
	Content   string
	Status    MessageStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}
