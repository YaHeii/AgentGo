package provider

import "context"

type StreamEvent struct {
	Type              StreamEventType
	TextDelta         string
	ReasoningDelta    string
	RefusalDelta      string
	ToolCallDelta     *ToolCallDelta
	ToolCall          *ToolCall
	Usage             *Usage
	StopReason        StopReason
	SystemFingerprint string
	Err               error
}

type StreamEventType string

const (
	StreamEventTextDelta         StreamEventType = "text_delta"
	StreamEventReasoningDelta    StreamEventType = "reasoning_delta"
	StreamEventRefusalDelta      StreamEventType = "refusal_delta"
	StreamEventToolCallDelta     StreamEventType = "tool_call_delta"
	StreamEventToolCallCompleted StreamEventType = "tool_call_completed"
	StreamEventUsageAvailable    StreamEventType = "usage_available"
	StreamEventTurnFinished      StreamEventType = "turn_finished"
	StreamEventProviderError     StreamEventType = "provider_error"
)

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

type ToolCallDelta struct {
	Index          int
	ID             string
	NameDelta      string
	ArgumentsDelta string
}

type ToolCall struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}

type StreamingLLM interface {
	StreamChat(ctx context.Context, req Request) <-chan StreamEvent
}
