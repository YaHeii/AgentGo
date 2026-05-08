package agent

import (
	"context"
	"errors"

	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
)

type QueryRunner interface {
	RunQuery(ctx context.Context, params QueryParams) (QueryResult, error)
	Events() <-chan Event
}

type QueryParams struct {
	SessionID string
	Prompt    string
}

type QueryResult struct {
	UserMessageID      string
	AssistantMessageID string
}

type MessageStore interface {
	Create(ctx context.Context, sessionID string, params message.CreateMessageParams) (message.Message, error)
	Update(ctx context.Context, message message.Message) error
	List(ctx context.Context, sessionID string) ([]message.Message, error)
}

type MessageQueryRunner struct {
	messages MessageStore
	llm      provider.StreamingLLM
	bus      bus.Bus[Event]
	events   <-chan Event
}

func NewMessageQueryRunner(messages MessageStore, llm provider.StreamingLLM) *MessageQueryRunner {
	b := bus.NewBus[Event](32)
	return &MessageQueryRunner{
		messages: messages,
		llm:      llm,
		bus:      b,
		events:   b.Subscribe(context.Background()),
	}
}

func (r *MessageQueryRunner) RunQuery(ctx context.Context, params QueryParams) (QueryResult, error) {
	userMessage, err := r.messages.Create(ctx, params.SessionID, message.CreateMessageParams{
		Kind:   message.KindUser,
		Origin: message.OriginHuman,
		Status: message.StatusComplete,
		Parts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: params.Prompt,
			},
		},
	})
	if err != nil {
		return QueryResult{}, err
	}

	assistantMessage, err := r.messages.Create(ctx, params.SessionID, message.CreateMessageParams{
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

	history, err := r.messages.List(ctx, params.SessionID)
	if err != nil {
		return QueryResult{}, err
	}

	stream := r.llm.StreamChat(ctx, toProviderMessages(history, assistantMessage.ID))
	for event := range stream {
		if event.Err != nil {
			assistantMessage.Status = message.StatusFailed
			if errors.Is(event.Err, context.Canceled) {
				assistantMessage.Status = message.StatusCancelled
			}
			_ = r.messages.Update(ctx, assistantMessage)
			r.publish(QueryFailedEvent{
				SessionID:          params.SessionID,
				UserMessageID:      userMessage.ID,
				AssistantMessageID: assistantMessage.ID,
				Err:                event.Err,
			})
			return QueryResult{
				UserMessageID:      userMessage.ID,
				AssistantMessageID: assistantMessage.ID,
			}, event.Err
		}

		switch event.Type {
		case provider.StreamEventDelta:
			appendTextDelta(&assistantMessage, event.Delta)
			assistantMessage.Status = message.StatusStreaming
			if err := r.messages.Update(ctx, assistantMessage); err != nil {
				return QueryResult{}, err
			}
		case provider.StreamEventDone:
			assistantMessage.Status = message.StatusComplete
			if err := r.messages.Update(ctx, assistantMessage); err != nil {
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
		UserMessageID:      userMessage.ID,
		AssistantMessageID: assistantMessage.ID,
	}, nil
}

func (r *MessageQueryRunner) Events() <-chan Event {
	return r.events
}

func (r *MessageQueryRunner) publish(event Event) {
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
