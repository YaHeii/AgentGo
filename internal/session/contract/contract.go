package contract

import (
	"errors"
	"time"
)

var ErrSessionNotFound = errors.New("session: not found")

type Session struct {
	ID               string
	Title            string
	MessageCount     int64
	CompletionTokens int64
	CostMicros       int64
	SummaryMessageID string
	TodosJSON        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CreateSessionParams struct {
	ID               string
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
	Title            string
	MessageCount     int64
	CompletionTokens int64
	CostMicros       int64
	SummaryMessageID string
	TodosJSON        string
	UpdatedAt        time.Time
}
