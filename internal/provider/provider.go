package provider

import (
	"context"

	providercontract "github.com/YaHeii/agentGo/internal/provider/contract"
)

type TurnRunner interface {
	RunTurn(ctx context.Context, req providercontract.Request) (providercontract.TurnResult, error)
}

type streamClient interface {
	Stream(ctx context.Context, req providercontract.Request) <-chan StreamEvent
}
