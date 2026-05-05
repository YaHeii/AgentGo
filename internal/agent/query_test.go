package agent

import (
	"context"
	"testing"

	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/stretchr/testify/require"
)

func TestMessageQueryRunnerImplementsQueryRunner(t *testing.T) {
	t.Parallel()

	var runner QueryRunner = NewMessageQueryRunner(&stubMessageSender{})
	require.NotNil(t, runner)
}

func TestMessageQueryRunnerMapsQueryParamsToMessageSend(t *testing.T) {
	t.Parallel()

	sender := &stubMessageSender{
		result: message.SendMessageResult{
			User:      messageRecord("user-1"),
			Assistant: messageRecord("assistant-1"),
		},
	}

	runner := NewMessageQueryRunner(sender)

	result, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		Prompt:    "hello",
	})
	require.NoError(t, err)
	require.Equal(t, "session-1", sender.lastParams.SessionID)
	require.Equal(t, "hello", sender.lastParams.Prompt)
	require.Equal(t, "user-1", result.UserMessageID)
	require.Equal(t, "assistant-1", result.AssistantMessageID)
}

type stubMessageSender struct {
	lastParams message.SendMessageParams
	result     message.SendMessageResult
	err        error
}

func (s *stubMessageSender) SendMessage(_ context.Context, params message.SendMessageParams) (message.SendMessageResult, error) {
	s.lastParams = params
	return s.result, s.err
}

func messageRecord(id string) store.Message {
	return store.Message{
		ID: id,
	}
}
