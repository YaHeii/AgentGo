package agent

import (
	"context"
	"errors"

	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
)

type Runner interface {
	RunQuery(ctx context.Context, params QueryParams) (QueryResult, error)
	Events() <-chan Event
}

type MessagePort interface {
	Create(ctx context.Context, sessionID string, params message.CreateMessageParams) (message.Message, error)
	Update(ctx context.Context, message message.Message) error
	List(ctx context.Context, sessionID string) ([]message.Message, error)
}

type QueryLoop struct {
	config   QueryConfig
	deps     QueryDeps
	bus      bus.Bus[Event]
	events   <-chan Event
}

func NewQueryLoop(messages MessagePort, llm provider.StreamingLLM) *QueryLoop {
	b := bus.NewBus[Event](32)
	return &QueryLoop{
		config:   defaultQueryConfig(),
		deps:     defaultQueryDeps(messages, llm),
		bus:      b,
		events:   b.Subscribe(context.Background()),
	}
}

func (r *QueryLoop) RunQuery(ctx context.Context, params QueryParams) (QueryResult, error) {
	if r.config.MaxTurns <= 0 {
		return QueryResult{}, errors.New("agent: max turns must be greater than 0")
	}

	state := newLoopState(params).withTransition("state_initialized")

	userMessage, err := r.deps.Messages.Create(ctx, params.SessionID, message.CreateMessageParams{
		Kind:   message.KindUser,
		Origin: message.OriginHuman,
		Status: message.StatusComplete,
		Parts:  cloneInputParts(params.InputParts),
	})
	if err != nil {
		return QueryResult{}, err
	}
	state = state.replaceLastMessage(userMessage).withTransition("user_message_created")

	assistantMessage, err := r.deps.Messages.Create(ctx, params.SessionID, message.CreateMessageParams{
		Kind:   message.KindAssistant,
		Origin: message.OriginModel,
		Status: message.StatusStreaming,
		Parts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: "",
			},
		},
	})
	if err != nil {
		return QueryResult{}, err
	}
	state = state.appendMessage(assistantMessage).withTransition("assistant_placeholder_created")

	history, err := r.deps.Messages.List(ctx, params.SessionID)
	if err != nil {
		return QueryResult{}, err
	}
	state = state.withTransition("history_loaded")

	stream := r.deps.LLM.StreamChat(ctx, toProviderMessages(history, assistantMessage.ID))
	for event := range stream {
		if event.Err != nil {
			assistantMessage.Status = message.StatusFailed
			if errors.Is(event.Err, context.Canceled) {
				assistantMessage.Status = message.StatusCancelled
			}
			state = state.replaceLastMessage(assistantMessage).withTransition("stream_failed")
			_ = r.deps.Messages.Update(ctx, assistantMessage)
			r.publish(QueryFailedEvent{
				SessionID:          params.SessionID,
				UserMessageID:      userMessage.ID,
				AssistantMessageID: assistantMessage.ID,
				Err:                event.Err,
			})
			return QueryResult{
				SessionID:               params.SessionID,
				UserMessageID:           userMessage.ID,
				FinalAssistantMessageID: assistantMessage.ID,
				Turns:                   state.turnCount,
				FinishReason:            finishReasonFromError(event.Err),
			}, event.Err
		}

		switch event.Type {
		case provider.StreamEventDelta:
			appendTextDelta(&assistantMessage, event.Delta)
			assistantMessage.Status = message.StatusStreaming
			state = state.replaceLastMessage(assistantMessage).withTransition("assistant_delta_received")
			if err := r.deps.Messages.Update(ctx, assistantMessage); err != nil {
				return QueryResult{}, err
			}
		case provider.StreamEventDone:
			assistantMessage.Status = message.StatusComplete
			state = state.replaceLastMessage(assistantMessage).withTransition("assistant_completed")
			if err := r.deps.Messages.Update(ctx, assistantMessage); err != nil {
				return QueryResult{}, err
			}
		}
	}

	r.publish(QueryCompletedEvent{
		SessionID:          params.SessionID,
		UserMessageID:      userMessage.ID,
		AssistantMessageID: assistantMessage.ID,
	})

	return QueryResult{
		SessionID:               params.SessionID,
		UserMessageID:           userMessage.ID,
		FinalAssistantMessageID: assistantMessage.ID,
		Turns:                   state.turnCount,
		FinishReason:            FinishReasonCompleted,
	}, nil
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
