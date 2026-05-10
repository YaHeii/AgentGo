package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
)

type ConversationPort interface {
	ListHistory(ctx context.Context, sessionID string) ([]message.Message, error)
	CreateMessage(ctx context.Context, sessionID string, params message.CreateMessageParams) (message.Message, error)
	UpdateMessage(ctx context.Context, sessionID string, msg message.Message) error
}

type QueryLoop struct {
	config QueryConfig
	deps   QueryDeps
	bus    bus.Bus[Event]
	events <-chan Event
}

func NewQueryLoop(conversation ConversationPort, llm provider.StreamingLLM) *QueryLoop {
	b := bus.NewBus[Event](32)
	return &QueryLoop{
		config: defaultQueryConfig(),
		deps:   defaultQueryDeps(conversation, llm),
		bus:    b,
		events: b.Subscribe(context.Background()),
	}
}

func (r *QueryLoop) RunQuery(ctx context.Context, params QueryParams) (QueryResult, error) {
	if r.config.MaxTurns <= 0 {
		return QueryResult{}, errors.New("agent: max turns must be greater than 0")
	}

	state := newLoopState(params).withTransition("state_initialized")

	userMessage, err := r.deps.Conversation.CreateMessage(ctx, params.SessionID, message.CreateMessageParams{
		Kind:   message.KindUser,
		Status: message.StatusComplete,
		Parts:  cloneInputParts(params.InputParts),
	})
	if err != nil {
		return QueryResult{}, err
	}
	state = state.replaceLastMessage(userMessage).withTransition("user_message_created")

	for turn := 1; turn <= r.config.MaxTurns; turn++ {
		state = state.withTurnCount(turn).withTransition("turn_started")

		history, err := r.deps.Conversation.ListHistory(ctx, params.SessionID)
		if err != nil {
			return QueryResult{}, err
		}
		state = state.withMessages(history).withTransition("history_loaded")
		assistantMessage := newAssistantStreamMessage(params.SessionID, turn, r.deps.Now().UTC())
		state = state.appendMessage(assistantMessage).withTransition("assistant_stream_initialized")
		req := provider.Request{
			Messages: toProviderMessages(history, ""),
		}

		outcome, nextState, err := r.runTurn(ctx, state, req)
		if err != nil {
			return QueryResult{}, err
		}
		state = nextState

		switch outcome.finishReason {
		case FinishReasonAwaitingToolExecution:
			// TODO: Add ToolExecutor to QueryDeps and resume the loop after tool results are persisted.
			return QueryResult{
				SessionID:               params.SessionID,
				UserMessageID:           userMessage.ID,
				FinalAssistantMessageID: outcome.assistantMessage.ID,
				Turns:                   state.turnCount,
				FinishReason:            outcome.finishReason,
				PendingToolCalls:        append([]provider.ToolCall(nil), outcome.pendingToolCalls...),
			}, nil
		case FinishReasonCompleted:
			if outcome.stopReason == provider.StopReasonLength && turn < r.config.MaxTurns {
				continue
			}

			r.publish(QueryCompletedEvent{
				SessionID:          params.SessionID,
				UserMessageID:      userMessage.ID,
				AssistantMessageID: outcome.assistantMessage.ID,
			})

			return QueryResult{
				SessionID:               params.SessionID,
				UserMessageID:           userMessage.ID,
				FinalAssistantMessageID: outcome.assistantMessage.ID,
				Turns:                   state.turnCount,
				FinishReason:            FinishReasonCompleted,
				PendingToolCalls:        append([]provider.ToolCall(nil), outcome.pendingToolCalls...),
			}, nil
		}
	}

	lastAssistant, _ := latestAssistantMessage(state.messages)
	return QueryResult{
		SessionID:               params.SessionID,
		UserMessageID:           userMessage.ID,
		FinalAssistantMessageID: lastAssistant.ID,
		Turns:                   state.turnCount,
		FinishReason:            FinishReasonCompleted,
	}, nil
}

func (r *QueryLoop) RunPrompt(ctx context.Context, sessionID string, prompt string) error {
	_, err := r.RunQuery(ctx, QueryParams{
		SessionID: sessionID,
		InputParts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: prompt,
			},
		},
	})
	return err
}

func (r *QueryLoop) Events() <-chan Event {
	return r.events
}

func (r *QueryLoop) publish(event Event) {
	if r.bus == nil {
		return
	}

	r.bus.Publish(event)
}

func toProviderMessages(messages []message.Message, skipID string) []provider.Message {
	out := make([]provider.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.ID == skipID && textOf(msg) == "" {
			continue
		}
		out = append(out, provider.Message{
			Role:    providerRole(msg.Kind),
			Content: textOf(msg),
		})
	}
	return out
}

func providerRole(kind message.Kind) string {
	switch kind {
	case message.KindAssistant:
		return "assistant"
	case message.KindUser:
		return "user"
	default:
		return "system"
	}
}

func textOf(msg message.Message) string {
	for _, part := range msg.Parts {
		if part.Type == message.PartTypeText {
			return part.Text
		}
	}
	return ""
}

func appendTextDelta(msg *message.Message, delta string) {
	for i := range msg.Parts {
		if msg.Parts[i].Type == message.PartTypeText {
			msg.Parts[i].Text += delta
			return
		}
	}

	msg.Parts = append(msg.Parts, message.Part{
		Type: message.PartTypeText,
		Text: delta,
	})
}

func cloneInputParts(parts []message.Part) []message.Part {
	if len(parts) == 0 {
		return nil
	}

	cloned := make([]message.Part, len(parts))
	copy(cloned, parts)
	return cloned
}

func finishReasonFromError(err error) FinishReason {
	if errors.Is(err, context.Canceled) {
		return FinishReasonCancelled
	}

	return FinishReasonFailed
}

type turnOutcome struct {
	assistantMessage message.Message
	persistedMessage message.Message
	finishReason     FinishReason
	stopReason       provider.StopReason
	pendingToolCalls []provider.ToolCall
}

func (r *QueryLoop) runTurn(ctx context.Context, state loopState, req provider.Request) (turnOutcome, loopState, error) {
	assistantMessage, ok := latestAssistantMessage(state.messages)
	if !ok {
		return turnOutcome{}, state, errors.New("agent: assistant message not found")
	}

	pendingToolCalls := make([]provider.ToolCall, 0)
	stream := r.deps.LLM.StreamChat(ctx, req)
	for event := range stream {
		if event.Type == provider.StreamEventProviderError {
			assistantMessage.Status = message.StatusFailed
			if errors.Is(event.Err, context.Canceled) {
				assistantMessage.Status = message.StatusCancelled
			}
			finishedAt := r.deps.Now().UTC()
			assistantMessage.UpdatedAt = finishedAt
			state = state.replaceLastMessage(assistantMessage).withTransition("stream_failed")
			if err := r.deps.Conversation.UpdateMessage(ctx, assistantMessage.SessionID, assistantMessage); err != nil {
				return turnOutcome{}, state, err
			}

			systemMessage, err := r.deps.Conversation.CreateMessage(ctx, assistantMessage.SessionID, message.CreateMessageParams{
				Kind:       message.KindSystem,
				Status:     assistantMessage.Status,
				FinishedAt: finishedAt,
				Parts: []message.Part{
					{
						Type: message.PartTypeText,
						Text: buildProviderErrorText(event.Err),
					},
				},
				System: &message.SystemPayload{
					Subtype: "provider_error",
					Level:   "error",
				},
			})
			if err != nil {
				return turnOutcome{}, state, err
			}
			r.publish(QueryFailedEvent{
				SessionID:          systemMessage.SessionID,
				UserMessageID:      firstUserMessageID(state.messages),
				AssistantMessageID: systemMessage.ID,
				Err:                event.Err,
			})
			return turnOutcome{
				assistantMessage: assistantMessage,
				persistedMessage: systemMessage,
				finishReason:     finishReasonFromError(event.Err),
			}, state, event.Err
		}

		switch event.Type {
		case provider.StreamEventTextDelta:
			appendTextDelta(&assistantMessage, event.TextDelta)
			assistantMessage.Status = message.StatusStreaming
			assistantMessage.UpdatedAt = r.deps.Now().UTC()
			state = state.replaceLastMessage(assistantMessage).withTransition("assistant_delta_received")
			if err := r.deps.Conversation.UpdateMessage(ctx, assistantMessage.SessionID, assistantMessage); err != nil {
				return turnOutcome{}, state, err
			}
		case provider.StreamEventReasoningDelta:
			appendThinkingDelta(&assistantMessage, event.ReasoningDelta)
			assistantMessage.Status = message.StatusStreaming
			assistantMessage.UpdatedAt = r.deps.Now().UTC()
			state = state.replaceLastMessage(assistantMessage).withTransition("assistant_reasoning_received")
			if err := r.deps.Conversation.UpdateMessage(ctx, assistantMessage.SessionID, assistantMessage); err != nil {
				return turnOutcome{}, state, err
			}
		case provider.StreamEventRefusalDelta:
			appendTextDelta(&assistantMessage, event.RefusalDelta)
			assistantMessage.Status = message.StatusStreaming
			assistantMessage.UpdatedAt = r.deps.Now().UTC()
			state = state.replaceLastMessage(assistantMessage).withTransition("assistant_refusal_received")
			if err := r.deps.Conversation.UpdateMessage(ctx, assistantMessage.SessionID, assistantMessage); err != nil {
				return turnOutcome{}, state, err
			}
		case provider.StreamEventToolCallCompleted:
			if event.ToolCall != nil {
				pendingToolCalls = append(pendingToolCalls, *event.ToolCall)
				appendToolCallPart(&assistantMessage, *event.ToolCall)
				assistantMessage.Status = message.StatusStreaming
				assistantMessage.UpdatedAt = r.deps.Now().UTC()
				state = state.replaceLastMessage(assistantMessage)
				state = state.withTransition("tool_call_completed")
			}
		case provider.StreamEventTurnFinished:
			assistantMessage.Status = message.StatusComplete
			finishedAt := r.deps.Now().UTC()
			assistantMessage.UpdatedAt = finishedAt
			state = state.replaceLastMessage(assistantMessage)

			persistedAssistant, err := r.deps.Conversation.CreateMessage(ctx, assistantMessage.SessionID, message.CreateMessageParams{
				ID:               assistantMessage.ID,
				Kind:             assistantMessage.Kind,
				Status:           message.StatusComplete,
				FinishedAt:       finishedAt,
				IsCompactSummary: assistantMessage.Flags.IsCompactSummary,
				Flags:            assistantMessage.Flags,
				Parts:            cloneInputParts(assistantMessage.Parts),
				System:           assistantMessage.System,
				Progress:         assistantMessage.Progress,
			})
			if err != nil {
				return turnOutcome{}, state, err
			}

			if event.StopReason == provider.StopReasonToolCalls {
				state = state.withTransition("awaiting_tool_execution")
				if err := r.deps.Conversation.UpdateMessage(ctx, persistedAssistant.SessionID, persistedAssistant); err != nil {
					return turnOutcome{}, state, err
				}
				return turnOutcome{
					assistantMessage: assistantMessage,
					persistedMessage: persistedAssistant,
					finishReason:     FinishReasonAwaitingToolExecution,
					stopReason:       event.StopReason,
					pendingToolCalls: append([]provider.ToolCall(nil), pendingToolCalls...),
				}, state, nil
			}

			state = state.withTransition("assistant_completed")
			if err := r.deps.Conversation.UpdateMessage(ctx, persistedAssistant.SessionID, persistedAssistant); err != nil {
				return turnOutcome{}, state, err
			}
			return turnOutcome{
				assistantMessage: assistantMessage,
				persistedMessage: persistedAssistant,
				finishReason:     FinishReasonCompleted,
				stopReason:       event.StopReason,
				pendingToolCalls: append([]provider.ToolCall(nil), pendingToolCalls...),
			}, state, nil
		}
	}

	return turnOutcome{
		assistantMessage: assistantMessage,
		finishReason:     FinishReasonCompleted,
	}, state, nil
}

func appendThinkingDelta(msg *message.Message, delta string) {
	for i := range msg.Parts {
		if msg.Parts[i].Type == message.PartTypeThinking && msg.Parts[i].Thinking != nil {
			msg.Parts[i].Thinking.Content += delta
			return
		}
	}

	msg.Parts = append(msg.Parts, message.Part{
		Type: message.PartTypeThinking,
		Thinking: &message.ThinkingPart{
			Content: delta,
		},
	})
}

func latestAssistantMessage(messages []message.Message) (message.Message, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Kind == message.KindAssistant {
			return cloneMessage(messages[i]), true
		}
	}
	return message.Message{}, false
}

func firstUserMessageID(messages []message.Message) string {
	for _, msg := range messages {
		if msg.Kind == message.KindUser {
			return msg.ID
		}
	}
	return ""
}

func newAssistantStreamMessage(sessionID string, turn int, now time.Time) message.Message {
	return message.Message{
		ID:        fmt.Sprintf("assistant-turn-%d-%d", turn, now.UnixNano()),
		SessionID: sessionID,
		Kind:      message.KindAssistant,
		Status:    message.StatusStreaming,
		CreatedAt: now,
		UpdatedAt: now,
		Parts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: "",
			},
		},
	}
}

// TODO:should be refacotr
func appendToolCallPart(msg *message.Message, call provider.ToolCall) {
	msg.Parts = append(msg.Parts, message.Part{
		Type: message.PartTypeToolCall,
		ToolCall: &message.ToolCallPart{
			ID:     call.ID,
			Name:   call.Name,
			Input:  call.Arguments,
			Status: "completed",
		},
	})
}

func buildProviderErrorText(err error) string {
	if err == nil {
		return "provider error"
	}
	return "provider error: " + err.Error()
}
