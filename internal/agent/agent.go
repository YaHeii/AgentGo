package agent

import (
	"context"
	"time"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/tool"
)


// QueryResult is the terminal snapshot returned when a query finishes successfully.
type QueryResult struct {
	SessionID               string
	UserMessageID           string
	FinalAssistantMessageID string
	Turns                   int
	FinishReason            FinishReason
	PendingToolCalls        []provider.ToolCall
}

// QueryConfig stores stable runtime limits and policy knobs for a query runner.
type QueryConfig struct {
	MaxTurns int
}

// QueryDeps groups the concrete collaborators required by the query runner.
type QueryDeps struct {
	App        appStore
	Provider   providerStore
	Now        func() time.Time
	dispatcher app.Dispatcher
}

// FinishReason describes why a query loop reached its terminal state.
type FinishReason string

const (
	FinishReasonCompleted             FinishReason = "completed"
	FinishReasonFailed                FinishReason = "failed"
	FinishReasonCancelled             FinishReason = "cancelled"
	FinishReasonAwaitingToolExecution FinishReason = "awaiting_tool_execution"
)

type providerStore interface {
	RunTurn(ctx context.Context, req provider.Request) (provider.TurnResult, error)
}

type appStore interface {
	ListHistory(ctx context.Context, sessionID string) ([]message.Message, error)
	CreateMessage(ctx context.Context, params message.CreateMessageParams) (message.Message, error)
	ListTools(ctx context.Context, permissionLevel tool.SecurityLevel) []tool.Metadata
	CallTools(ctx context.Context, req tool.BatchRequest) ([]tool.ToolResult, error)
}
