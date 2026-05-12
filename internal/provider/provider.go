package provider

import (
	"context"
	"encoding/json"
)

type TurnRunner interface {
	RunTurn(ctx context.Context, req Request) (TurnResult, error)
}

type Request struct {
	Messages []Message
	Tools    []ToolDefinition
	Context  RequestContext
}

type Message struct {
	Role       Role
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type RequestContext struct {
	Temperature     *float32
	MaxOutputTokens *int
}

type TurnResult struct {
	Text              string
	Reasoning         string
	Refusal           string
	ToolCalls         []ToolCall
	Usage             *Usage
	StopReason        StopReason
	SystemFingerprint string
}

type streamClient interface {
	Stream(ctx context.Context, req Request) <-chan StreamEvent
}
