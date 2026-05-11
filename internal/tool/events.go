package tool

type ToolResultEvent struct {
	Status ToolCalslStatus
	Result ToolResult
	Err    error
}

type ToolCalslStatus string

const (
	ToolCalslStatusStarted   ToolCalslStatus = "started"
	ToolCalslStatusDelta     ToolCalslStatus = "delta"
	ToolCalslStatusCompleted ToolCalslStatus = "completed"
	ToolCalslStatusFailed    ToolCalslStatus = "failed"
)
