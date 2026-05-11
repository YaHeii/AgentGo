package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
)

type QueryLoop struct {
	config QueryConfig
	deps   QueryDeps
}

func NewQueryLoop(conversation sessionStore, llm provider.StreamingLLM, d app.Dispatcher) *QueryLoop {
	return &QueryLoop{
		config: QueryConfig{
			MaxTurns: 1,
		},
		deps: QueryDeps{
			Conversation: conversation,
			LLM:          llm,
			Now:          time.Now,
			dispatcher:   d,
		},
	}
}

// TODO: TOdelete
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

func (r *QueryLoop) RunQuery(ctx context.Context, params QueryParams) (QueryResult, error) {
	// start with Precheck
	// 1. MaxTurn Check
	if r.config.MaxTurns <= 0 {
		return QueryResult{}, errors.New("agent: max turns must be greater than 0")
	}

	//
	var inputParts []message.Part
	if len(params.InputParts) > 0 {
		inputParts = make([]message.Part, len(params.InputParts))
		copy(inputParts, params.InputParts)
	}

	userMessage, err := r.deps.Conversation.CreateMessage(ctx, params.SessionID, message.CreateMessageParams{
		Kind:  message.KindUser,
		Parts: inputParts,
	}, r.deps.dispatcher)
	if err != nil {
		return QueryResult{}, err
	}

	state := newLoopState(params)
	state.Messages = []message.Message{userMessage}
	state.Transition = "user_message_created"
	if r.deps.dispatcher != nil {
		r.deps.dispatcher.Dispatch(app.BaseEvent{
			T: app.EventAgent,
			Payload: QueryEvent{
				Status: QueryStatusStarted,
				State:  copyLoopState(state),
				Err:    nil,
			},
		})
	}

	for {
		if state.TurnCount > r.config.MaxTurns {
			break
		}

		state = copyLoopState(state)
		state.Transition = "turn_started"

		// TODO: Add per-turn preprocessing once tool loop ownership is finalized.
		history, err := r.deps.Conversation.ListHistory(ctx, params.SessionID, r.deps.dispatcher)
		if err != nil {
			return QueryResult{}, err
		}
		state.Messages = history
		state.Transition = "history_loaded"

		assistantNow := r.deps.Now().UTC()
		assistantMessage := message.Message{
			ID:        fmt.Sprintf("assistant-turn-%d-%d", state.TurnCount, assistantNow.UnixNano()),
			SessionID: params.SessionID,
			Kind:      message.KindAssistant,
			CreatedAt: assistantNow,
			UpdatedAt: assistantNow,
			Parts: []message.Part{
				{
					Type: message.PartTypeText,
					Text: "",
				},
			},
		}
		state.Messages = append(state.Messages, assistantMessage)
		state.Transition = "assistant_stream_initialized"
		if r.deps.dispatcher != nil {
			r.deps.dispatcher.Dispatch(app.BaseEvent{
				T: app.EventAgent,
				Payload: QueryEvent{
					Status: QueryStatusDelta,
					State:  copyLoopState(state),
					Err:    nil,
				},
			})
		}

		req := provider.Request{
			SessionID: params.SessionID,
		}

		outcome, nextState, err := r.runTurn(ctx, state, req)
		state = nextState
		if err != nil {
			if r.deps.dispatcher != nil {
				r.deps.dispatcher.Dispatch(app.BaseEvent{
					T: app.EventAgent,
					Payload: QueryEvent{
						Status: QueryStatusFailed,
						State:  copyLoopState(state),
						Err:    err,
					},
				})
			}
			return QueryResult{}, err
		}

		switch outcome.finishReason {
		case FinishReasonAwaitingToolExecution:
			return QueryResult{
				SessionID:               params.SessionID,
				UserMessageID:           userMessage.ID,
				FinalAssistantMessageID: outcome.assistantMessage.ID,
				Turns:                   state.TurnCount,
				FinishReason:            outcome.finishReason,
				PendingToolCalls:        append([]provider.ToolCall(nil), outcome.pendingToolCalls...),
			}, nil
		case FinishReasonCompleted:
			if outcome.stopReason == provider.StopReasonLength && state.TurnCount < r.config.MaxTurns {
				state = copyLoopState(state)
				state.TurnCount++
				continue
			}

			if r.deps.dispatcher != nil {
				r.deps.dispatcher.Dispatch(app.BaseEvent{
					T: app.EventAgent,
					Payload: QueryEvent{
						Status: QueryStatusCompleted,
						State:  copyLoopState(state),
						Err:    nil,
					},
				})
			}
			return QueryResult{
				SessionID:               params.SessionID,
				UserMessageID:           userMessage.ID,
				FinalAssistantMessageID: outcome.assistantMessage.ID,
				Turns:                   state.TurnCount,
				FinishReason:            FinishReasonCompleted,
				PendingToolCalls:        append([]provider.ToolCall(nil), outcome.pendingToolCalls...),
			}, nil
		}
	}

	lastAssistantID := ""
	for i := len(state.Messages) - 1; i >= 0; i-- {
		if state.Messages[i].Kind == message.KindAssistant {
			lastAssistantID = state.Messages[i].ID
			break
		}
	}
	if r.deps.dispatcher != nil {
		r.deps.dispatcher.Dispatch(app.BaseEvent{
			T: app.EventAgent,
			Payload: QueryEvent{
				Status: QueryStatusCompleted,
				State:  copyLoopState(state),
				Err:    nil,
			},
		})
	}
	return QueryResult{
		SessionID:               params.SessionID,
		UserMessageID:           userMessage.ID,
		FinalAssistantMessageID: lastAssistantID,
		Turns:                   state.TurnCount,
		FinishReason:            FinishReasonCompleted,
	}, nil
}

func (r *QueryLoop) runTurn(ctx context.Context, state LoopState, req provider.Request) (turnOutcome, LoopState, error) {
	state = copyLoopState(state)

	assistantIndex := -1
	for i := len(state.Messages) - 1; i >= 0; i-- {
		if state.Messages[i].Kind == message.KindAssistant {
			assistantIndex = i
			break
		}
	}
	if assistantIndex < 0 {
		return turnOutcome{}, state, errors.New("agent: assistant message not found")
	}
	assistantMessage := state.Messages[assistantIndex]

	pendingToolCalls := make([]provider.ToolCall, 0)
	stream := r.deps.LLM.StreamChat(ctx, req)
	
	for event := range stream {
		if event.Type == provider.StreamEventProviderError {
			finishedAt := r.deps.Now().UTC()
			assistantMessage.UpdatedAt = finishedAt
			state = copyLoopState(state)
			state.Messages[assistantIndex] = assistantMessage
			state.Transition = "stream_failed"
			if r.deps.dispatcher != nil {
				r.deps.dispatcher.Dispatch(app.BaseEvent{
					T: app.EventAgent,
					Payload: QueryEvent{
						Status: QueryStatusFailed,
						State:  copyLoopState(state),
						Err:    event.Err,
					},
				})
			}

			providerErrorText := "provider error"
			if event.Err != nil {
				providerErrorText = "provider error: " + event.Err.Error()
			}

			systemMessage, err := r.deps.Conversation.CreateMessage(ctx, assistantMessage.SessionID, message.CreateMessageParams{
				Kind:       message.KindSystem,
				FinishedAt: finishedAt,
				Parts: []message.Part{
					{
						Type: message.PartTypeText,
						Text: providerErrorText,
					},
				},
				System: &message.SystemPayload{
					Subtype: "provider_error",
					Level:   "error",
				},
			}, r.deps.dispatcher)
			if err != nil {
				return turnOutcome{}, state, err
			}

			state = copyLoopState(state)
			state.Messages = append(state.Messages, systemMessage)
			state.Transition = "provider_error_recorded"
			finishReason := FinishReasonFailed
			if errors.Is(event.Err, context.Canceled) {
				finishReason = FinishReasonCancelled
			}
			return turnOutcome{
				assistantMessage: assistantMessage,
				persistedMessage: systemMessage,
				finishReason:     finishReason,
			}, state, event.Err
		}

		switch event.Type {
		case provider.StreamEventTextDelta:
			textUpdated := false
			for i := range assistantMessage.Parts {
				if assistantMessage.Parts[i].Type == message.PartTypeText {
					assistantMessage.Parts[i].Text += event.TextDelta
					textUpdated = true
					break
				}
			}
			if !textUpdated {
				assistantMessage.Parts = append(assistantMessage.Parts, message.Part{
					Type: message.PartTypeText,
					Text: event.TextDelta,
				})
			}
			assistantMessage.UpdatedAt = r.deps.Now().UTC()
			state = copyLoopState(state)
			state.Messages[assistantIndex] = assistantMessage
			state.Transition = "assistant_delta_received"
			if r.deps.dispatcher != nil {
				r.deps.dispatcher.Dispatch(app.BaseEvent{
					T: app.EventAgent,
					Payload: QueryEvent{
						Status: QueryStatusDelta,
						State:  copyLoopState(state),
						Err:    nil,
					},
				})
			}
		case provider.StreamEventReasoningDelta:
			reasoningUpdated := false
			for i := range assistantMessage.Parts {
				if assistantMessage.Parts[i].Type == message.PartTypeThinking && assistantMessage.Parts[i].Thinking != nil {
					assistantMessage.Parts[i].Thinking.Content += event.ReasoningDelta
					reasoningUpdated = true
					break
				}
			}
			if !reasoningUpdated {
				assistantMessage.Parts = append(assistantMessage.Parts, message.Part{
					Type: message.PartTypeThinking,
					Thinking: &message.ThinkingPart{
						Content: event.ReasoningDelta,
					},
				})
			}
			assistantMessage.UpdatedAt = r.deps.Now().UTC()
			state = copyLoopState(state)
			state.Messages[assistantIndex] = assistantMessage
			state.Transition = "assistant_reasoning_received"
			if r.deps.dispatcher != nil {
				r.deps.dispatcher.Dispatch(app.BaseEvent{
					T: app.EventAgent,
					Payload: QueryEvent{
						Status: QueryStatusDelta,
						State:  copyLoopState(state),
						Err:    nil,
					},
				})
			}
		case provider.StreamEventRefusalDelta:
			refusalUpdated := false
			for i := range assistantMessage.Parts {
				if assistantMessage.Parts[i].Type == message.PartTypeText {
					assistantMessage.Parts[i].Text += event.RefusalDelta
					refusalUpdated = true
					break
				}
			}
			if !refusalUpdated {
				assistantMessage.Parts = append(assistantMessage.Parts, message.Part{
					Type: message.PartTypeText,
					Text: event.RefusalDelta,
				})
			}
			assistantMessage.UpdatedAt = r.deps.Now().UTC()
			state = copyLoopState(state)
			state.Messages[assistantIndex] = assistantMessage
			state.Transition = "assistant_refusal_received"
			if r.deps.dispatcher != nil {
				r.deps.dispatcher.Dispatch(app.BaseEvent{
					T: app.EventAgent,
					Payload: QueryEvent{
						Status: QueryStatusDelta,
						State:  copyLoopState(state),
						Err:    nil,
					},
				})
			}
		case provider.StreamEventToolCallCompleted:
			if event.ToolCall != nil {
				pendingToolCalls = append(pendingToolCalls, *event.ToolCall)
				assistantMessage.Parts = append(assistantMessage.Parts, message.Part{
					Type: message.PartTypeToolCall,
					ToolCall: &message.ToolCallPart{
						ID:     event.ToolCall.ID,
						Name:   event.ToolCall.Name,
						Input:  event.ToolCall.Arguments,
						Status: "completed",
					},
				})
				assistantMessage.UpdatedAt = r.deps.Now().UTC()
				state = copyLoopState(state)
				state.Messages[assistantIndex] = assistantMessage
				state.Transition = "tool_call_completed"
				if r.deps.dispatcher != nil {
					r.deps.dispatcher.Dispatch(app.BaseEvent{
						T: app.EventAgent,
						Payload: QueryEvent{
							Status: QueryStatusDelta,
							State:  copyLoopState(state),
							Err:    nil,
						},
					})
				}
			}
		case provider.StreamEventTurnFinished:
			finishedAt := r.deps.Now().UTC()
			assistantMessage.UpdatedAt = finishedAt
			state = copyLoopState(state)
			state.Messages[assistantIndex] = assistantMessage

			var persistedParts []message.Part
			if len(assistantMessage.Parts) > 0 {
				persistedParts = make([]message.Part, len(assistantMessage.Parts))
				for i := range assistantMessage.Parts {
					persistedParts[i] = assistantMessage.Parts[i]
					if assistantMessage.Parts[i].Image != nil {
						imagePart := *assistantMessage.Parts[i].Image
						persistedParts[i].Image = &imagePart
					}
					if assistantMessage.Parts[i].ToolCall != nil {
						toolCallPart := *assistantMessage.Parts[i].ToolCall
						persistedParts[i].ToolCall = &toolCallPart
					}
					if assistantMessage.Parts[i].ToolResult != nil {
						toolResultPart := *assistantMessage.Parts[i].ToolResult
						persistedParts[i].ToolResult = &toolResultPart
					}
					if assistantMessage.Parts[i].Thinking != nil {
						thinkingPart := *assistantMessage.Parts[i].Thinking
						persistedParts[i].Thinking = &thinkingPart
					}
					if assistantMessage.Parts[i].Attachment != nil {
						attachmentPart := *assistantMessage.Parts[i].Attachment
						persistedParts[i].Attachment = &attachmentPart
					}
					if assistantMessage.Parts[i].Summary != nil {
						summaryPart := *assistantMessage.Parts[i].Summary
						persistedParts[i].Summary = &summaryPart
					}
				}
			}

			persistedAssistant, err := r.deps.Conversation.CreateMessage(ctx, assistantMessage.SessionID, message.CreateMessageParams{
				ID:               assistantMessage.ID,
				Kind:             assistantMessage.Kind,
				FinishedAt:       finishedAt,
				IsCompactSummary: assistantMessage.Flags.IsCompactSummary,
				Flags:            assistantMessage.Flags,
				Parts:            persistedParts,
				System:           assistantMessage.System,
				Progress:         assistantMessage.Progress,
			}, r.deps.dispatcher)
			if err != nil {
				return turnOutcome{}, state, err
			}

			state = copyLoopState(state)
			state.Messages[assistantIndex] = persistedAssistant
			if event.StopReason == provider.StopReasonToolCalls {
				state.Transition = "awaiting_tool_execution"
				if r.deps.dispatcher != nil {
					r.deps.dispatcher.Dispatch(app.BaseEvent{
						T: app.EventAgent,
						Payload: QueryEvent{
							Status: QueryStatusDelta,
							State:  copyLoopState(state),
							Err:    nil,
						},
					})
				}
				return turnOutcome{
					assistantMessage: persistedAssistant,
					persistedMessage: persistedAssistant,
					finishReason:     FinishReasonAwaitingToolExecution,
					stopReason:       event.StopReason,
					pendingToolCalls: append([]provider.ToolCall(nil), pendingToolCalls...),
				}, state, nil
			}

			state.Transition = "assistant_completed"
			if r.deps.dispatcher != nil {
				r.deps.dispatcher.Dispatch(app.BaseEvent{
					T: app.EventAgent,
					Payload: QueryEvent{
						Status: QueryStatusDelta,
						State:  copyLoopState(state),
						Err:    nil,
					},
				})
			}
			return turnOutcome{
				assistantMessage: persistedAssistant,
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
