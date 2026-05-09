package store

import (
	"errors"
	"time"
)

var (
	ErrSessionNotFound = errors.New("store: session not found")
	ErrMessageNotFound = errors.New("store: message not found")
)

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
}

type Message struct {
	ID               string
	SessionID        string
	Kind             string
	Provider         string
	FinishedAt       time.Time
	IsCompactSummary bool
	MessageJSON      string
}

type CreateSessionParams struct {
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
}

type UpdateSessionParams struct {
	ID               string
	ParentSessionID  string
	Title            string
	MessageCount     int64
	CompletionTokens int64
	CostMicros       int64
	SummaryMessageID string
	TodosJSON        string
	UpdatedAt        time.Time
}

type CreateMessageParams struct {
	ID               string
	SessionID        string
	Kind             string
	Provider         string
	FinishedAt       time.Time
	IsCompactSummary bool
	MessageJSON      string
}

// TODO: Consider the necessity of draft tables
type SaveDraftParams struct {
	SessionID string
	Content   string
	UpdatedAt time.Time
}
