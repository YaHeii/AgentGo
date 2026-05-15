package contract

import (
	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

type Request struct {
	Messages []messagecontract.Message
	Tools    []toolcontract.Metadata
	Context  RequestContext
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

type ToolCall struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}

type StopReason string

const (
	StopReasonStop          StopReason = "stop"
	StopReasonLength        StopReason = "length"
	StopReasonFunctionCall  StopReason = "function_call"
	StopReasonToolCalls     StopReason = "tool_calls"
	StopReasonContentFilter StopReason = "content_filter"
	StopReasonCancelled     StopReason = "cancelled"
	StopReasonUnknown       StopReason = "unknown"
)

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}
