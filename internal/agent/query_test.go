package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/stretchr/testify/require"
)

func TestMessageQueryRunnerImplementsQueryRunner(t *testing.T) {
	t.Parallel()

	var runner QueryRunner = NewMessageQueryRunner(&stubMessageStore{}, &stubStreamingLLM{})
	require.NotNil(t, runner)
}

func TestMessageQueryRunnerCreatesMessagesAndStreamsAssistantReply(t *testing.T) {
	t.Parallel()

	store := &stubMessageStore{
		listResult: []message.Message{
			messageRecord("user-1", message.KindUser, "hello"),
			messageRecord("assistant-1", message.KindAssistant, ""),
		},
	}
	llm := &stubStreamingLLM{
		events: []provider.StreamEvent{
			{Type: provider.StreamEventDelta, Delta: "hel"},
			{Type: provider.StreamEventDelta, Delta: "lo"},
			{Type: provider.StreamEventDone},
		},
	}

	runner := NewMessageQueryRunner(store, llm)

	result, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		Prompt:    "hello",
	})
	require.NoError(t, err)
	require.Equal(t, "user-1", result.UserMessageID)
	require.Equal(t, "assistant-1", result.AssistantMessageID)
	require.Len(t, store.created, 2)
	require.Len(t, store.updated, 3)
	require.Equal(t, "hello", llm.calls[0][0].Content)
	require.Equal(t, "hello", textOf(store.updated[len(store.updated)-1]))
	require.Equal(t, message.StatusComplete, store.updated[len(store.updated)-1].Status)
}

func TestMessageQueryRunnerMarksAssistantFailedOnStreamError(t *testing.T) {
	t.Parallel()

	store := &stubMessageStore{
		listResult: []message.Message{
			messageRecord("user-1", message.KindUser, "hello"),
			messageRecord("assistant-1", message.KindAssistant, ""),
		},
	}
	llm := &stubStreamingLLM{
		events: []provider.StreamEvent{
			{Type: provider.StreamEventDelta, Delta: "par"},
			{Err: errors.New("stream failed")},
		},
	}

	runner := NewMessageQueryRunner(store, llm)

	_, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		Prompt:    "hello",
	})
	require.Error(t, err)
	require.Len(t, store.updated, 2)
	require.Equal(t, message.StatusFailed, store.updated[len(store.updated)-1].Status)
	require.Equal(t, "par", textOf(store.updated[len(store.updated)-1]))
}

type stubMessageStore struct {
	created    []message.CreateMessageParams
	updated    []message.Message
	listResult []message.Message
}

func (s *stubMessageStore) Create(_ context.Context, sessionID string, params message.CreateMessageParams) (message.Message, error) {
	s.created = append(s.created, params)
	id := "user-1"
	if params.Kind == message.KindAssistant {
		id = "assistant-1"
	}
	return message.Message{
		ID:        id,
		SessionID: sessionID,
		Kind:      params.Kind,
		Origin:    params.Origin,
		Status:    params.Status,
		Parts:     params.Parts,
	}, nil
}

func (s *stubMessageStore) Update(_ context.Context, msg message.Message) error {
	s.updated = append(s.updated, msg)
	return nil
}

func (s *stubMessageStore) List(_ context.Context, _ string) ([]message.Message, error) {
	copied := append([]message.Message(nil), s.listResult...)
	return copied, nil
}

type stubStreamingLLM struct {
	events []provider.StreamEvent
	calls  [][]provider.Message
}

func (s *stubStreamingLLM) StreamChat(_ context.Context, messages []provider.Message) <-chan provider.StreamEvent {
	s.calls = append(s.calls, append([]provider.Message(nil), messages...))
	ch := make(chan provider.StreamEvent, len(s.events))
	for _, event := range s.events {
		ch <- event
	}
	close(ch)
	return ch
}

func messageRecord(id string, kind message.Kind, text string) message.Message {
	return message.Message{
		ID:     id,
		Kind:   kind,
		Status: message.StatusComplete,
		Parts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: text,
			},
		},
	}
}
