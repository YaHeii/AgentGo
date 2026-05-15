package tool

import (
	"context"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

type Tool interface {
	Metadata() toolcontract.Metadata
	Execute(ctx context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult
}

type MCPStore interface {
	Transport() // define the mcp trans method
}
