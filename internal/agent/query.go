package agent

import (
	"context"

	"github.com/YaHeii/agentGo/internal/message"
)

type QueryRunner interface {
	RunQuery(ctx context.Context, params QueryParams) (QueryResult, error)
}

type QueryParams struct {
	SessionID string
	Prompt    string
}

type QueryResult struct {
	UserMessageID      string
	AssistantMessageID string
}

type MessageSender interface {
	SendMessage(ctx context.Context, params message.SendMessageParams) (message.SendMessageResult, error)
}

type MessageQueryRunner struct {
	messages MessageSender
}

func NewMessageQueryRunner(messages MessageSender) *MessageQueryRunner {
	return &MessageQueryRunner{messages: messages}
}

func (r *MessageQueryRunner) RunQuery(ctx context.Context, params QueryParams) (QueryResult, error) {
	result, err := r.messages.SendMessage(ctx, message.SendMessageParams{
		SessionID: params.SessionID,
		Prompt:    params.Prompt,
	})
	if err != nil {
		return QueryResult{}, err
	}

	return QueryResult{
		UserMessageID:      result.User.ID,
		AssistantMessageID: result.Assistant.ID,
	}, nil
}
