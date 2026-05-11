package agent

type QueryEvent struct {
	Status QueryStatus
	State  LoopState
	Err    error
}

type QueryStatus string

const (
	QueryStatusStarted   QueryStatus = "started"
	QueryStatusDelta     QueryStatus = "delta"
	QueryStatusCompleted QueryStatus = "completed"
	QueryStatusFailed    QueryStatus = "failed"
)
