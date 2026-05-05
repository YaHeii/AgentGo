package message

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/YaHeii/agentGo/internal/bus"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/store"
)

type MessageService struct {
	store   store.Store
	llm     provider.StreamingLLM
	bus     bus.Bus[Event]
	nowFunc func() time.Time
}

func NewMessageService(st store.Store, llm provider.StreamingLLM, nowFunc func() time.Time) *MessageService {
	if nowFunc == nil {
		nowFunc = time.Now
	}

	return &MessageService{
		store:   st,
		llm:     llm,
		bus:     bus.NewBus[Event](128),
		nowFunc: nowFunc,
	}
}

var _ Service = (*MessageService)(nil)

func (s *MessageService) SendMessage(ctx context.Context, params SendMessageParams) (SendMessageResult, error) {
	prompt := strings.TrimSpace(params.Prompt)
	if prompt == "" {
		return SendMessageResult{}, errors.New("prompt cannot be empty")
	}

	now := s.nowFunc().UTC()
	userMessage := store.Message{
		ID:        "user-" + now.Format(time.RFC3339Nano),
		SessionID: params.SessionID,
		Role:      "user",
		Content:   prompt,
		Status:    store.MessageStatusComplete,
		CreatedAt: now,
		UpdatedAt: now,
	}
	assistantMessage := store.Message{
		ID:        "assistant-" + now.Format(time.RFC3339Nano),
		SessionID: params.SessionID,
		Role:      "assistant",
		Content:   "",
		Status:    store.MessageStatusStreaming,
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := s.store.WithinTx(ctx, func(tx store.TxStore) error {
		if _, err := tx.CreateMessage(ctx, store.CreateMessageParams{
			ID:        userMessage.ID,
			SessionID: userMessage.SessionID,
			Role:      userMessage.Role,
			Content:   userMessage.Content,
			Status:    userMessage.Status,
			CreatedAt: userMessage.CreatedAt,
			UpdatedAt: userMessage.UpdatedAt,
		}); err != nil {
			return err
		}

		if _, err := tx.CreateMessage(ctx, store.CreateMessageParams{
			ID:        assistantMessage.ID,
			SessionID: assistantMessage.SessionID,
			Role:      assistantMessage.Role,
			Content:   assistantMessage.Content,
			Status:    assistantMessage.Status,
			CreatedAt: assistantMessage.CreatedAt,
			UpdatedAt: assistantMessage.UpdatedAt,
		}); err != nil {
			return err
		}

		if err := tx.DeleteDraft(ctx, params.SessionID); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return SendMessageResult{}, err
	}

	s.publish(MessageCreatedEvent{Message: toMessage(userMessage)})
	s.publish(MessageCreatedEvent{Message: toMessage(assistantMessage)})

	history, err := s.store.ListMessages(ctx, params.SessionID)
	if err != nil {
		return SendMessageResult{}, fmt.Errorf("list session history: %w", err)
	}

	messages := make([]provider.Message, 0, len(history))
	for _, msg := range history {
		if msg.ID == assistantMessage.ID && msg.Content == "" {
			continue
		}

		messages = append(messages, provider.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	stream := s.llm.StreamChat(ctx, messages)

	for streamEvent := range stream {
		if streamEvent.Err != nil {
			assistantMessage.Status = store.MessageStatusFailed
			if errors.Is(streamEvent.Err, context.Canceled) {
				assistantMessage.Status = store.MessageStatusCancelled
			}
			assistantMessage.UpdatedAt = s.nowFunc().UTC()

			if _, err := s.store.UpdateMessage(ctx, store.UpdateMessageParams{
				ID:        assistantMessage.ID,
				Content:   assistantMessage.Content,
				Status:    assistantMessage.Status,
				UpdatedAt: assistantMessage.UpdatedAt,
			}); err != nil {
				return SendMessageResult{}, err
			}

			if assistantMessage.Status == store.MessageStatusCancelled {
				s.publish(MessageCancelledEvent{Message: toMessage(assistantMessage), Err: streamEvent.Err})
			} else {
				s.publish(MessageFailedEvent{Message: toMessage(assistantMessage), Err: streamEvent.Err})
			}

			return SendMessageResult{
				User:      toMessage(userMessage),
				Assistant: toMessage(assistantMessage),
			}, streamEvent.Err
		}

		switch streamEvent.Type {
		case provider.StreamEventDelta:
			assistantMessage.Content += streamEvent.Delta
			assistantMessage.UpdatedAt = s.nowFunc().UTC()
			if _, err := s.store.UpdateMessage(ctx, store.UpdateMessageParams{
				ID:        assistantMessage.ID,
				Content:   assistantMessage.Content,
				Status:    assistantMessage.Status,
				UpdatedAt: assistantMessage.UpdatedAt,
			}); err != nil {
				return SendMessageResult{}, fmt.Errorf("update assistant message: %w", err)
			}

			s.publish(MessageDeltaEvent{
				Message: toMessage(assistantMessage),
				Delta:   streamEvent.Delta,
			})
		case provider.StreamEventDone:
			assistantMessage.Status = store.MessageStatusComplete
			assistantMessage.UpdatedAt = s.nowFunc().UTC()
			if _, err := s.store.UpdateMessage(ctx, store.UpdateMessageParams{
				ID:        assistantMessage.ID,
				Content:   assistantMessage.Content,
				Status:    assistantMessage.Status,
				UpdatedAt: assistantMessage.UpdatedAt,
			}); err != nil {
				return SendMessageResult{}, fmt.Errorf("complete assistant message: %w", err)
			}

			s.publish(MessageCompletedEvent{Message: toMessage(assistantMessage)})
		}
	}

	return SendMessageResult{
		User:      toMessage(userMessage),
		Assistant: toMessage(assistantMessage),
	}, nil
}

func (s *MessageService) Events() <-chan Event {
	return s.bus.Events()
}

func (s *MessageService) publish(evt Event) {
	if s.bus == nil {
		return
	}

	s.bus.Publish(evt)
}

func toMessage(msg store.Message) Message {
	return Message{
		ID:        msg.ID,
		SessionID: msg.SessionID,
		Role:      msg.Role,
		Content:   msg.Content,
		Status:    Status(msg.Status),
		CreatedAt: msg.CreatedAt,
		UpdatedAt: msg.UpdatedAt,
	}
}
