package tool

import toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"

// Deprecated
type ToolResultEvent struct {
	Status toolcontract.ToolCallStatus
	Result toolcontract.ToolResult
	Err    error
}
