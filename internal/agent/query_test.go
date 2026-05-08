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

	var runner Runner = NewQueryLoop(&stubMessageStore{}, &stubStreamingLLM{})
	require.NotNil(t, runner)
}

func TestNewMessageQueryRunnerSeedsConfigAndDeps(t *testing.T) {
	t.Parallel()

	store := &stubMessageStore{}
	llm := &stubStreamingLLM{}

	runner := NewQueryLoop(store, llm)

	require.Equal(t, 1, runner.config.MaxTurns)
	require.Same(t, store, runner.deps.Messages)
	require.Same(t, llm, runner.deps.LLM)
}

func TestQueryLoopRunQueryUsesInjectedDeps(t *testing.T) {
	t.Parallel()

	originalStore := &stubMessageStore{}
	originalLLM := &stubStreamingLLM{}

	depsStore := &stubMessageStore{
		listResult: []message.Message{
			messageRecord("user-1", message.KindUser, "hello"),
			messageRecord("assistant-1", message.KindAssistant, ""),
		},
	}
	depsLLM := &stubStreamingLLM{
		events: []provider.StreamEvent{
			{Type: provider.StreamEventDone},
		},
	}

	runner := NewQueryLoop(originalStore, originalLLM)
	runner.deps.Messages = depsStore
	runner.deps.LLM = depsLLM

	_, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: "hello",
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, originalStore.created, 0)
	require.Len(t, depsStore.created, 2)
	require.Len(t, originalLLM.calls, 0)
	require.Len(t, depsLLM.calls, 1)
}

func TestQueryLoopRejectsNonPositiveMaxTurns(t *testing.T) {
	t.Parallel()

	runner := NewQueryLoop(&stubMessageStore{}, &stubStreamingLLM{})
	runner.config.MaxTurns = 0

	_, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: "hello",
			},
		},
	})
	require.EqualError(t, err, "agent: max turns must be greater than 0")
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

	runner := NewQueryLoop(store, llm)

	result, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: "hello",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "session-1", result.SessionID)
	require.Equal(t, "user-1", result.UserMessageID)
	require.Equal(t, "assistant-1", result.FinalAssistantMessageID)
	require.Equal(t, 1, result.Turns)
	require.Equal(t, FinishReasonCompleted, result.FinishReason)
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

	runner := NewQueryLoop(store, llm)

	_, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: "hello",
			},
		},
	})
	require.Error(t, err)
	require.Len(t, store.updated, 2)
	require.Equal(t, message.StatusFailed, store.updated[len(store.updated)-1].Status)
	require.Equal(t, "par", textOf(store.updated[len(store.updated)-1]))
}

func TestNewLoopStateSeedsMessagesAndTurnCount(t *testing.T) {
	t.Parallel()

	state := newLoopState(QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: "hello",
			},
		},
	})

	require.Len(t, state.messages, 1)
	require.Equal(t, 1, state.turnCount)
	require.Equal(t, "hello", textOf(state.messages[0]))
}

func TestLoopStateWithTransitionReturnsNewSnapshot(t *testing.T) {
	t.Parallel()

	state := newLoopState(QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: "hello",
			},
		},
	})

	next := state.withTransition("assistant_delta")

	require.Empty(t, state.transition)
	require.Equal(t, "assistant_delta", next.transition)
}

func TestLoopStateAppendMessageDoesNotMutateOriginalSnapshot(t *testing.T) {
	t.Parallel()

	state := newLoopState(QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: "hello",
			},
		},
	})

	next := state.appendMessage(messageRecord("assistant-1", message.KindAssistant, "world"))

	require.Len(t, state.messages, 1)
	require.Len(t, next.messages, 2)
	require.Equal(t, "hello", textOf(state.messages[0]))
	require.Equal(t, "world", textOf(next.messages[1]))
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
