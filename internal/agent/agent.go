package agent

import (
	"context"

	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	providercontract "github.com/YaHeii/agentGo/internal/provider/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

//TODO： move to appstore
type providerStore interface {
	RunTurn(ctx context.Context, req providercontract.Request) (providercontract.TurnResult, error)
}

type appStore interface {
	ListHistory(ctx context.Context, sessionID string) ([]messagecontract.Message, error)
	CreateMessage(ctx context.Context, params messagecontract.CreateMessageParams) (messagecontract.Message, error)
	ListTools(ctx context.Context, permissionLevel toolcontract.SecurityLevel) []toolcontract.Metadata
	CallTools(ctx context.Context, req toolcontract.BatchRequest) ([]toolcontract.ToolResult, error)
}
