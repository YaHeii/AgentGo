package tool

import (
	"context"
	"encoding/json"
)

type Tool interface {
	Metadata() Metadata
	Execute(ctx context.Context, req ToolCallRequest) ToolResult
}

type Metadata struct {
	Name              string
	Description       string
	Parameters        json.RawMessage
	Enabled           bool
	SecurityLevel     SecurityLevel
	IsConcurrencySafe bool
	Requirements      ResourceRequirement
}

type SecurityLevel string

const (
	SafeLevel      SecurityLevel = "safe"
	AttentionLevel SecurityLevel = "attention"
	DangerLevel    SecurityLevel = "danger"
)

type ResourceRequirement uint32

const (
	RequireNone       ResourceRequirement = 0
	RequireWorkingDir ResourceRequirement = 1 << iota
	RequireWorkspaceRoot
	RequireNetwork
)

type MCPStore interface {
	Transport() // define the mcp trans method
}

type ToolCallContext struct {
	SessionID     string
	TurnID        string
	WorkspaceRoot string
	WorkingDir    string
}

type ToolCallRequest struct {
	ToolCallID string
	Name       string
	Arguments  json.RawMessage
	Context    ToolCallContext
}

type BatchRequest struct {
	Calls []ToolCallRequest
}

func NewToolCallRequest(
	toolCallID string,
	name string,
	args json.RawMessage,
	callCtx ToolCallContext,
) ToolCallRequest {
	return ToolCallRequest{
		ToolCallID: toolCallID,
		Name:       name,
		Arguments:  args,
		Context:    callCtx,
	}
}

func NewBatchRequest(calls ...ToolCallRequest) BatchRequest {
	copied := append([]ToolCallRequest(nil), calls...)
	return BatchRequest{Calls: copied}
}
