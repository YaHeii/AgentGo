package contract

import (
	"encoding/json"
)

type ToolCallContext struct {
	SessionID     string
	TurnID        string
	WorkspaceRoot string
	WorkingDir    string
}

type ToolCallRequest struct {
	ToolCallID      string
	Name            string
	Arguments       json.RawMessage
	PermissionLevel SecurityLevel
	Context         ToolCallContext
}

type BatchRequest struct {
	Calls []ToolCallRequest
}

type ToolResult struct {
	ToolCallID string
	Name       string
	Status     ToolCallStatus
	Content    string
	Metadata   map[string]any
	Err        error
}

type ToolCallStatus string

const (
	StatusSuccess ToolCallStatus = "success"

	StatusSyntaxRepaired ToolCallStatus = "syntax_repaired"

	StatusValidationFailed ToolCallStatus = "validation_failed"

	StatusExecutionError ToolCallStatus = "execution_error"

	StatusSystemError ToolCallStatus = "system_error"
)

type Metadata struct {
	Name              string
	Description       string
	Parameters        json.RawMessage
	Enabled           bool
	SecurityLevel     SecurityLevel
	IsConcurrencySafe bool
	Requirements      ResourceRequirement
}

type SecurityLevel int

const (
	SafeLevel SecurityLevel = iota
	AttentionLevel
	DangerLevel
)

type ResourceRequirement uint32

const (
	RequireNone       ResourceRequirement = 0
	RequireWorkingDir ResourceRequirement = 1 << iota
	RequireWorkspaceRoot
	RequireNetwork
)
