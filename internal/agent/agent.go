package agent

import (
	"context"
	"errors"
	"time"

	"github.com/YaHeii/agentGo/internal/app"
	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	providercontract "github.com/YaHeii/agentGo/internal/provider/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

// QueryResult is the terminal snapshot returned when a query finishes successfully.
type QueryResult struct {
	SessionID               string
	UserMessageID           string
	FinalAssistantMessageID string
	Turns                   int
	FinishReason            FinishReason
	PendingToolCalls        []providercontract.ToolCall
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
	RunTurn(ctx context.Context, req providercontract.Request) (providercontract.TurnResult, error)
}

type appStore interface {
	ListHistory(ctx context.Context, sessionID string) ([]messagecontract.Message, error)
	CreateMessage(ctx context.Context, params messagecontract.CreateMessageParams) (messagecontract.Message, error)
	ListTools(ctx context.Context, permissionLevel toolcontract.SecurityLevel) []toolcontract.Metadata
	CallTools(ctx context.Context, req toolcontract.BatchRequest) ([]toolcontract.ToolResult, error)
}

type Service struct {
	loop *QueryLoop
}

func NewService(loop *QueryLoop) *Service {
	return &Service{loop: loop}
}

func (s *Service) RunQuery(ctx context.Context, sessionID string, prompt string) error {
	if s == nil || s.loop == nil {
		return errors.New("agent: query loop is required")
	}
	_, err := s.loop.RunQuery(ctx, sessionID, prompt)
	return err
}
