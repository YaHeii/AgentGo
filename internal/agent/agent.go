package agent

import (
	"time"

	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
)

// QueryParams contains the minimal external inputs required to run a query.
type QueryParams struct {
	SessionID  string
	InputParts []message.Part

	// TODO: Add CanUseTool once tool capability gating is designed.
	// TODO: Add UserContext once caller-supplied user context injection is designed.
	// TODO: Add SystemContext once system prompt layering is designed.
	// TODO: Add ToolUseContext once tool loop state ownership is designed.
}

// QueryConfig stores stable runtime limits and policy knobs for a query runner.
type QueryConfig struct {
	MaxTurns int
}

// QueryDeps groups the concrete collaborators required by the query runner.
type QueryDeps struct {
	Conversation ConversationPort
	LLM          provider.StreamingLLM
	Now          func() time.Time
}

// QueryResult is the terminal snapshot returned when a query finishes successfully.
type QueryResult struct {
	SessionID               string
	UserMessageID           string
	FinalAssistantMessageID string
	Turns                   int
	FinishReason            FinishReason
	PendingToolCalls        []provider.ToolCall
}

// FinishReason describes why a query loop reached its terminal state.
type FinishReason string

const (
	FinishReasonCompleted             FinishReason = "completed"
	FinishReasonFailed                FinishReason = "failed"
	FinishReasonCancelled             FinishReason = "cancelled"
	FinishReasonAwaitingToolExecution FinishReason = "awaiting_tool_execution"
)

type autoCompactTracking struct{}

type pendingToolUseSummary struct{}
