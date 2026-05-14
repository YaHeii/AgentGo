package agent

import (
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
)

// LoopState stores the minimal mutable state required to continue a query loop.
// TODO：select fields which need to be exported
type LoopState struct {
	Messages           []message.Message
	TurnCount          int
	Transition         string // Transition records the latest control-flow transition inside the query loop.
	AssistantMessageID string
	FinishReason       FinishReason
	StopReason         provider.StopReason
	PendingToolCalls   []provider.ToolCall
}

// LoopState function
func newLoopState(params QueryParams) LoopState {
	return LoopState{
		Messages: []message.Message{
			{
				SessionID: params.SessionID,
				Kind:      message.KindUser,
				Parts:     append([]message.Part(nil), params.InputParts...),
			},
		},
		TurnCount: 1,
	}
}

// TO Deep copy loopstate
func copyLoopState(state LoopState) LoopState {
	copied := state
	copied.PendingToolCalls = append([]provider.ToolCall(nil), state.PendingToolCalls...)
	if len(state.Messages) == 0 {
		copied.Messages = nil
		return copied
	}

	copied.Messages = make([]message.Message, len(state.Messages))
	for i := range state.Messages {
		copied.Messages[i] = state.Messages[i]
		if len(state.Messages[i].Parts) == 0 {
			copied.Messages[i].Parts = nil
			continue
		}

		copied.Messages[i].Parts = make([]message.Part, len(state.Messages[i].Parts))
		for j := range state.Messages[i].Parts {
			copied.Messages[i].Parts[j] = state.Messages[i].Parts[j]
			if state.Messages[i].Parts[j].Image != nil {
				imagePart := *state.Messages[i].Parts[j].Image
				copied.Messages[i].Parts[j].Image = &imagePart
			}
			if state.Messages[i].Parts[j].ToolCall != nil {
				toolCallPart := *state.Messages[i].Parts[j].ToolCall
				copied.Messages[i].Parts[j].ToolCall = &toolCallPart
			}
			if state.Messages[i].Parts[j].ToolResult != nil {
				toolResultPart := *state.Messages[i].Parts[j].ToolResult
				copied.Messages[i].Parts[j].ToolResult = &toolResultPart
			}
			if state.Messages[i].Parts[j].Thinking != nil {
				thinkingPart := *state.Messages[i].Parts[j].Thinking
				copied.Messages[i].Parts[j].Thinking = &thinkingPart
			}
			if state.Messages[i].Parts[j].Attachment != nil {
				attachmentPart := *state.Messages[i].Parts[j].Attachment
				copied.Messages[i].Parts[j].Attachment = &attachmentPart
			}
			if state.Messages[i].Parts[j].Summary != nil {
				summaryPart := *state.Messages[i].Parts[j].Summary
				copied.Messages[i].Parts[j].Summary = &summaryPart
			}
		}
	}

	return copied
}
