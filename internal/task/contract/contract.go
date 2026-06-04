package contract

import (
	"errors"
	"time"
)

var ErrTaskNotFound = errors.New("task: not found")

type Status string

const (
	StatusRunning  Status = "running"
	StatusComplete Status = "complete"
	StatusFailed   Status = "failed"
)

type Task struct {
	SubagentSessionID string
	ParentSessionID   string
	Kind              string
	Status            Status
	InputPayloadJSON  string
	ProgressPayloadJSON string
	ResultPayloadJSON string
	ErrorMessage      string
	CreatedAt         time.Time
	CompletedAt       *time.Time
}

type CreateTaskParams struct {
	SubagentSessionID  string
	ParentSessionID    string
	Kind               string
	InputPayloadJSON   string
	ProgressPayloadJSON string
	ResultPayloadJSON  string
	ErrorMessage       string
	CreatedAt          time.Time
}

type UpdateTaskProgressParams struct {
	SubagentSessionID  string
	ProgressPayloadJSON string
}

type CompleteTaskParams struct {
	SubagentSessionID string
	ResultPayloadJSON string
	CompletedAt       time.Time
}

type FailTaskParams struct {
	SubagentSessionID string
	ErrorMessage      string
	CompletedAt       time.Time
}
