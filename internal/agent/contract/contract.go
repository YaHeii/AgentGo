package contract

// QueryResult is the terminal snapshot returned when a query finishes successfully.
type QueryResult struct {
	SessionID               string
	UserMessageID           string
	FinalAssistantMessageID int
	Turns                   int
	FinishReason            LoopStatus
	PendingToolCalls        []ToolCall
}

// LoopStatus describes why a query loop reached its terminal state.
// Use the `stopreason` field to perform the determination.
type LoopStatus string

const (
	LoopStarted           LoopStatus = "started"
	LoopCompleted         LoopStatus = "completed"
	LoopFinishFailed      LoopStatus = "failed"
	FinishReasonCancelled LoopStatus = "cancelled"
	LoopAwaitingTool      LoopStatus = "awaiting_tool_execution"
)

type ToolCall struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}
