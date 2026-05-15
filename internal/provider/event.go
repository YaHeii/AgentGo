package provider

import providercontract "github.com/YaHeii/agentGo/internal/provider/contract"

type StreamEvent struct {
	Type              StreamEventType
	TextDelta         string
	ReasoningDelta    string
	RefusalDelta      string
	ToolCallDelta     *ToolCallDelta
	Usage             *providercontract.Usage
	StopReason        providercontract.StopReason
	SystemFingerprint string
	Err               error
}

type StreamEventType string

const (
	StreamEventTextDelta      StreamEventType = "text_delta"
	StreamEventReasoningDelta StreamEventType = "reasoning_delta"
	StreamEventRefusalDelta   StreamEventType = "refusal_delta"
	StreamEventToolCallDelta  StreamEventType = "tool_call_delta"
	StreamEventUsageAvailable StreamEventType = "usage_available"
	StreamEventTurnFinished   StreamEventType = "turn_finished"
	StreamEventProviderError  StreamEventType = "provider_error"
)

// TODO: Requires Optimization
type ToolCallDelta struct {
	Index          int
	ID             string
	NameDelta      string
	ArgumentsDelta string
}
