package app

type QueryFinishReason string

const (
	QueryFinishReasonCompleted             QueryFinishReason = "completed"
	QueryFinishReasonFailed                QueryFinishReason = "failed"
	QueryFinishReasonCancelled             QueryFinishReason = "cancelled"
	QueryFinishReasonAwaitingToolExecution QueryFinishReason = "awaiting_tool_execution"
)

type ToolCall struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}

type SendMessageParams struct {
	SessionID string
	Prompt    string
}

type SendMessageResult struct {
	User      Message
	Assistant Message
	FinishReason     QueryFinishReason
	PendingToolCalls []ToolCall
}
