package agent

import (
	"log/slog"
	"time"

	agentcontract "github.com/YaHeii/agentGo/internal/agent/contract"
	"github.com/YaHeii/agentGo/internal/app"
	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	providercontract "github.com/YaHeii/agentGo/internal/provider/contract"
)

// LoopState stores the minimal mutable state required to continue a query loop.
type LoopState struct {
	Messages           []messagecontract.Message
	TurnCount          int
	Transition         string // DEBUG: Transition records the latest control-flow transition inside the query loop.
	AssistantMessageID string
	LoopStatus         agentcontract.LoopStatus
	ProviderStopReason providercontract.StopReason
	PendingToolCalls   []providercontract.ToolCall
	// stopHookActive
}

// QueryDeps groups the concrete collaborators required by the query runner.
type QueryDeps struct {
	App        appStore
	Provider   providerStore
	Now        func() time.Time
	dispatcher app.Dispatcher
}

func (l *LoopState) Continue(msg messagecontract.Message, transition string, finish agentcontract.LoopStatus) LoopState {
	return LoopState{
		Messages:   append(l.Messages, msg),
		TurnCount:  l.TurnCount + 1,
		Transition: transition,
		LoopStatus: finish,
	}
}

// LogValue implements the slog.LogValuer interface and hides sensitive fields.
func (l LoopState) LogValue() slog.Value {
	// To extract the name arr
	toolNames := make([]string, 0, len(l.PendingToolCalls))

	for _, tc := range l.PendingToolCalls {
		toolNames = append(toolNames, tc.Name)
	}
	return slog.GroupValue(
		slog.Int("id", l.TurnCount),
		slog.String("name", l.Transition),
		slog.String("StopReason", string(l.ProviderStopReason)),
		slog.Any("toolNames", toolNames),
	)
}

//	TODO: DELETE
// copyLoopState deep-copies mutable slices so event subscribers and later turns
// cannot observe in-place mutations from the active loop.
func copyLoopState(state LoopState) LoopState {
	copied := state
	copied.PendingToolCalls = append([]providercontract.ToolCall(nil), state.PendingToolCalls...)
	if len(state.Messages) == 0 {
		copied.Messages = nil
		return copied
	}

	copied.Messages = make([]messagecontract.Message, len(state.Messages))
	for i := range state.Messages {
		copied.Messages[i] = state.Messages[i]
		if len(state.Messages[i].Parts) == 0 {
			copied.Messages[i].Parts = nil
			continue
		}

		copied.Messages[i].Parts = make([]messagecontract.Part, len(state.Messages[i].Parts))
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
		}
	}

	return copied
}
