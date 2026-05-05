package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/store"
)

type Store interface {
	store.Store
	UpdateSession(ctx context.Context, params store.UpdateSessionParams) (store.Session, error)
}

type Service struct {
	store   Store
	llm     provider.StreamingLLM
	nowFunc func() time.Time
}

func NewService(st Store, llm provider.StreamingLLM, nowFunc func() time.Time) *Service {
	if nowFunc == nil {
		nowFunc = time.Now
	}

	return &Service{
		store:   st,
		llm:     llm,
		nowFunc: nowFunc,
	}
}

func (s *Service) SendMessage(ctx context.Context, params SendMessageParams) (SendMessageResult, error) {
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

	for event := range stream {
		if event.Err != nil {
			assistantMessage.Status = store.MessageStatusFailed
			if errors.Is(event.Err, context.Canceled) {
				assistantMessage.Status = store.MessageStatusCancelled
			}
			assistantMessage.UpdatedAt = s.nowFunc().UTC()
			_, _ = s.store.UpdateMessage(ctx, store.UpdateMessageParams{
				ID:        assistantMessage.ID,
				Content:   assistantMessage.Content,
				Status:    assistantMessage.Status,
				UpdatedAt: assistantMessage.UpdatedAt,
			})
			return SendMessageResult{
				User:      userMessage,
				Assistant: assistantMessage,
			}, event.Err
		}

		switch event.Type {
		case provider.StreamEventDelta:
			assistantMessage.Content += event.Delta
			assistantMessage.UpdatedAt = s.nowFunc().UTC()
			if _, err := s.store.UpdateMessage(ctx, store.UpdateMessageParams{
				ID:        assistantMessage.ID,
				Content:   assistantMessage.Content,
				Status:    assistantMessage.Status,
				UpdatedAt: assistantMessage.UpdatedAt,
			}); err != nil {
				return SendMessageResult{}, fmt.Errorf("update assistant message: %w", err)
			}
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
		}
	}

	return SendMessageResult{
		User:      userMessage,
		Assistant: assistantMessage,
	}, nil
}
