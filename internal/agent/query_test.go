package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/stretchr/testify/require"
)

func TestNewQueryLoopReturnsRunner(t *testing.T) {
	t.Parallel()

	runner := NewQueryLoop(&stubMessageStore{}, &stubStreamingLLM{})
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

	depsStore := &stubMessageStore{}
	depsLLM := &stubStreamingLLM{
		events: [][]provider.StreamEvent{
			{
				{Type: provider.StreamEventTurnFinished, StopReason: provider.StopReasonStop},
			},
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
	require.Len(t, depsLLM.calls[0].Messages, 1)
	require.Equal(t, "hello", depsLLM.calls[0].Messages[0].Content)
	require.Equal(t, message.StatusComplete, depsStore.created[1].Status)
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

	store := &stubMessageStore{}
	llm := &stubStreamingLLM{
		events: [][]provider.StreamEvent{
			{
				{Type: provider.StreamEventTextDelta, TextDelta: "hel"},
				{Type: provider.StreamEventTextDelta, TextDelta: "lo"},
				{Type: provider.StreamEventTurnFinished, StopReason: provider.StopReasonStop},
			},
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
	require.Equal(t, 1, result.Turns)
	require.Equal(t, FinishReasonCompleted, result.FinishReason)
	require.Len(t, store.created, 2)
	require.Len(t, store.updated, 3)
	require.Len(t, llm.calls[0].Messages, 1)
	require.Equal(t, "hello", llm.calls[0].Messages[0].Content)
	require.Equal(t, message.StatusComplete, store.created[1].Status)
	require.Equal(t, "hello", findTextPart(store.created[1].Parts))
	require.Equal(t, store.persisted[1].ID, result.FinalAssistantMessageID)
	require.Equal(t, "hello", textOf(store.updated[len(store.updated)-1]))
	require.Equal(t, message.StatusComplete, store.updated[len(store.updated)-1].Status)
}

func TestMessageQueryRunnerMarksAssistantFailedOnStreamError(t *testing.T) {
	t.Parallel()

	store := &stubMessageStore{}
	llm := &stubStreamingLLM{
		events: [][]provider.StreamEvent{
			{
				{Type: provider.StreamEventTextDelta, TextDelta: "par"},
				{Type: provider.StreamEventProviderError, Err: errors.New("stream failed")},
			},
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
	require.Len(t, store.created, 2)
	require.Len(t, store.updated, 2)
	require.Equal(t, message.KindSystem, store.created[1].Kind)
	require.Contains(t, findTextPart(store.created[1].Parts), "stream failed")
	require.Equal(t, message.StatusFailed, store.updated[len(store.updated)-1].Status)
	require.Equal(t, "par", textOf(store.updated[len(store.updated)-1]))
}

func TestQueryLoopMapsToolCallStopReasonToAwaitingExecution(t *testing.T) {
	t.Parallel()

	store := &stubMessageStore{}
	llm := &stubStreamingLLM{
		events: [][]provider.StreamEvent{
			{
				{
					Type: provider.StreamEventToolCallDelta,
					ToolCallDelta: &provider.ToolCallDelta{
						Index:          0,
						ID:             "call_1",
						NameDelta:      "search",
						ArgumentsDelta: "{\"q\":\"golang\"}",
					},
				},
				{
					Type: provider.StreamEventToolCallCompleted,
					ToolCall: &provider.ToolCall{
						Index:     0,
						ID:        "call_1",
						Name:      "search",
						Arguments: "{\"q\":\"golang\"}",
					},
				},
				{Type: provider.StreamEventTurnFinished, StopReason: provider.StopReasonToolCalls},
			},
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
	require.Equal(t, FinishReasonAwaitingToolExecution, result.FinishReason)
	require.Len(t, result.PendingToolCalls, 1)
	require.Equal(t, "call_1", result.PendingToolCalls[0].ID)
	require.Equal(t, "search", result.PendingToolCalls[0].Name)
	require.Len(t, store.created, 2)
	require.Equal(t, message.StatusComplete, store.created[1].Status)
	require.NotNil(t, findToolCallPart(store.created[1].Parts))
	require.Equal(t, "call_1", findToolCallPart(store.created[1].Parts).ID)
}

func TestQueryLoopStoresReasoningAndRefusalParts(t *testing.T) {
	t.Parallel()

	store := &stubMessageStore{}
	llm := &stubStreamingLLM{
		events: [][]provider.StreamEvent{
			{
				{Type: provider.StreamEventReasoningDelta, ReasoningDelta: "thinking"},
				{Type: provider.StreamEventRefusalDelta, RefusalDelta: "decline"},
				{Type: provider.StreamEventTurnFinished, StopReason: provider.StopReasonStop},
			},
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
	require.NoError(t, err)
	require.NotEmpty(t, store.updated)

	require.Len(t, store.created, 2)
	require.Equal(t, message.StatusComplete, store.created[1].Status)
	require.NotNil(t, findThinkingPart(store.created[1].Parts))
	require.Equal(t, "decline", findTextPart(store.created[1].Parts))

	final := store.updated[len(store.updated)-1]
	require.Equal(t, message.StatusComplete, final.Status)
	require.NotNil(t, findThinkingPart(final.Parts))
	require.Equal(t, "thinking", findThinkingPart(final.Parts).Content)
	require.Equal(t, "decline", findTextPart(final.Parts))
}

func TestQueryLoopStopsAtConfiguredMaxTurns(t *testing.T) {
	t.Parallel()

	store := &stubMessageStore{}
	llm := &stubStreamingLLM{
		events: [][]provider.StreamEvent{
			{
				{Type: provider.StreamEventTextDelta, TextDelta: "one"},
				{Type: provider.StreamEventTurnFinished, StopReason: provider.StopReasonLength},
			},
			{
				{Type: provider.StreamEventTurnFinished, StopReason: provider.StopReasonStop},
			},
		},
	}

	runner := NewQueryLoop(store, llm)
	runner.config.MaxTurns = 2

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
	require.Len(t, llm.calls, 2)
	require.Equal(t, 2, result.Turns)
	require.Equal(t, FinishReasonCompleted, result.FinishReason)
	require.Len(t, llm.calls[1].Messages, 2)
	require.Equal(t, "one", llm.calls[1].Messages[1].Content)
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
	persisted  []message.Message
}

func (s *stubMessageStore) Create(_ context.Context, sessionID string, params message.CreateMessageParams) (message.Message, error) {
	s.created = append(s.created, params)
	id := "user-1"
	switch params.Kind {
	case message.KindAssistant:
		id = "assistant-1"
	case message.KindSystem:
		id = "system-1"
	}
	if params.ID != "" {
		id = params.ID
	}
	msg := message.Message{
		ID:        id,
		SessionID: sessionID,
		Kind:      params.Kind,
		Status:    params.Status,
		Parts:     params.Parts,
		System:    params.System,
		Progress:  params.Progress,
	}
	s.persisted = append(s.persisted, msg)
	return msg, nil
}

func (s *stubMessageStore) Update(_ context.Context, msg message.Message) error {
	s.updated = append(s.updated, msg)
	return nil
}

func (s *stubMessageStore) List(_ context.Context, _ string) ([]message.Message, error) {
	copied := append([]message.Message(nil), s.listResult...)
	copied = append(copied, s.persisted...)
	return copied, nil
}

type stubStreamingLLM struct {
	events [][]provider.StreamEvent
	calls  []provider.Request
}

func (s *stubStreamingLLM) StreamChat(_ context.Context, req provider.Request) <-chan provider.StreamEvent {
	s.calls = append(s.calls, cloneProviderRequest(req))
	index := len(s.calls) - 1
	batch := []provider.StreamEvent(nil)
	if index < len(s.events) {
		batch = s.events[index]
	}
	ch := make(chan provider.StreamEvent, len(batch))
	for _, event := range batch {
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

func cloneProviderRequest(req provider.Request) provider.Request {
	cloned := provider.Request{}
	if len(req.Messages) > 0 {
		cloned.Messages = append([]provider.Message(nil), req.Messages...)
	}
	return cloned
}

func findThinkingPart(parts []message.Part) *message.ThinkingPart {
	for _, part := range parts {
		if part.Type == message.PartTypeThinking && part.Thinking != nil {
			return part.Thinking
		}
	}
	return nil
}

func findTextPart(parts []message.Part) string {
	for _, part := range parts {
		if part.Type == message.PartTypeText {
			return part.Text
		}
	}
	return ""
}

func findToolCallPart(parts []message.Part) *message.ToolCallPart {
	for _, part := range parts {
		if part.Type == message.PartTypeToolCall && part.ToolCall != nil {
			return part.ToolCall
		}
	}
	return nil
}
