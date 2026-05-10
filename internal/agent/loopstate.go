package agent

import (
	"time"

	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
)

// loopState stores the minimal mutable state required to continue a query loop.
type loopState struct {
	messages                     []message.Message
	maxOutputTokensOverride      *int
	autoCompactTracking          *autoCompactTracking
	stopHookActive               bool
	maxOutputTokensRecoveryCount int
	hasAttemptedReactiveCompact  bool
	turnCount                    int
	pendingToolUseSummary        *pendingToolUseSummary
	transition                   string // Transition records the latest control-flow transition inside the query loop.

	// TODO: Add toolUseContext once tool loop ownership is finalized.
	// TODO: Add richer transition metadata after recovery and compact paths exist.
}

// LoopState function
func newLoopState(params QueryParams) loopState {
	return loopState{
		messages: []message.Message{
			{
				SessionID: params.SessionID,
				Kind:      message.KindUser,
				Status:    message.StatusComplete,
				Parts:     append([]message.Part(nil), params.InputParts...),
			},
		},
		turnCount: 1,
	}
}

func (s loopState) withTransition(reason string) loopState {
	next := s
	next.transition = reason
	return next
}

func (s loopState) withMessages(messages []message.Message) loopState {
	next := s
	next.messages = cloneMessages(messages)
	return next
}

func (s loopState) appendMessage(msg message.Message) loopState {
	next := s
	next.messages = append(cloneMessages(s.messages), cloneMessage(msg))
	return next
}

func (s loopState) replaceLastMessage(msg message.Message) loopState {
	next := s
	next.messages = cloneMessages(s.messages)
	if len(next.messages) == 0 {
		next.messages = append(next.messages, cloneMessage(msg))
		return next
	}

	next.messages[len(next.messages)-1] = cloneMessage(msg)
	return next
}

func (s loopState) withTurnCount(turnCount int) loopState {
	next := s
	next.turnCount = turnCount
	return next
}

func defaultQueryConfig() QueryConfig {
	return QueryConfig{
		MaxTurns: 1,
	}
}

func defaultQueryDeps(conversation ConversationPort, llm provider.StreamingLLM) QueryDeps {
	return QueryDeps{
		Conversation: conversation,
		LLM:          llm,
		Now:          time.Now,
	}
}

func cloneMessages(messages []message.Message) []message.Message {
	if len(messages) == 0 {
		return nil
	}

	cloned := make([]message.Message, len(messages))
	for i := range messages {
		cloned[i] = cloneMessage(messages[i])
	}
	return cloned
}

func cloneMessage(msg message.Message) message.Message {
	cloned := msg
	cloned.Parts = cloneMessageParts(msg.Parts)
	return cloned
}

func cloneMessageParts(parts []message.Part) []message.Part {
	if len(parts) == 0 {
		return nil
	}

	cloned := make([]message.Part, len(parts))
	copy(cloned, parts)
	return cloned
}
