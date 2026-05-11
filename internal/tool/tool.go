package tool

import (
	"context"
	"encoding/json"
)

// tool interface
type Tool interface {
	Definition() Definition

	Execute(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

type Definition struct {
	Enabled           bool
	SecurityLevel     string
	isConcurrencySafe bool
}

type SecurityLevel string

const (
	SafeLevel      SecurityLevel = "safe"
	AttentionLevel SecurityLevel = "attention"
	DangerLevel    SecurityLevel = "danger"
)


type MCPStore interface{
 Transport() // define the mcp trans method
}