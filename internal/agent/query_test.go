package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/stretchr/testify/require"
)

func TestNewQueryLoopReturnsRunner(t *testing.T) {
	t.Parallel()

	runner := NewQueryLoop(&stubSessionConversationPort{}, &stubStreamingLLM{}, app.NewDispatcher(16))
	require.NotNil(t, runner)
}

func TestNewQueryLoopSeedsConfigAndDeps(t *testing.T) {
	t.Parallel()

	store := &stubSessionConversationPort{}
	llm := &stubStreamingLLM{}
	dispatcher := app.NewDispatcher(16)

	runner := NewQueryLoop(store, llm, dispatcher)

	require.Equal(t, 1, runner.config.MaxTurns)
	require.Same(t, store, runner.deps.Conversation)
	require.Same(t, llm, runner.deps.LLM)
	require.Same(t, dispatcher, runner.deps.dispatcher)
}

func TestQueryLoopRunQueryUsesInjectedDeps(t *testing.T) {
	t.Parallel()

	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())

	originalStore := &stubSessionConversationPort{}
	originalLLM := &stubStreamingLLM{}

	depsStore := &stubSessionConversationPort{}
	depsLLM := &stubStreamingLLM{
		events: [][]provider.StreamEvent{
			{
				{Type: provider.StreamEventTurnFinished, StopReason: provider.StopReasonStop},
			},
		},
	}

	runner := NewQueryLoop(originalStore, originalLLM, dispatcher)
	runner.deps.Conversation = depsStore
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
	require.Equal(t, "session-1", depsLLM.calls[0].SessionID)

	gotStarted := <-events
	require.Equal(t, app.EventAgent, gotStarted.Type())
	started, ok := gotStarted.Data().(QueryEvent)
	require.True(t, ok)
	require.Equal(t, QueryStatusStarted, started.Status)
}

func TestQueryLoopRejectsNonPositiveMaxTurns(t *testing.T) {
	t.Parallel()

	runner := NewQueryLoop(&stubSessionConversationPort{}, &stubStreamingLLM{}, app.NewDispatcher(16))
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

func TestQueryLoopCreatesMessagesAndStreamsAssistantReply(t *testing.T) {
	t.Parallel()

	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())
	store := &stubSessionConversationPort{}
	llm := &stubStreamingLLM{
		events: [][]provider.StreamEvent{
			{
				{Type: provider.StreamEventTextDelta, TextDelta: "hel"},
				{Type: provider.StreamEventTextDelta, TextDelta: "lo"},
				{Type: provider.StreamEventTurnFinished, StopReason: provider.StopReasonStop},
			},
		},
	}

	runner := NewQueryLoop(store, llm, dispatcher)

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
	require.Equal(t, []string{"session-1"}, store.hydratedSessionIDs)
	require.Equal(t, "session-1", llm.calls[0].SessionID)
	require.Equal(t, "hello", findTextPart(store.created[1].Parts))
	require.Equal(t, store.persisted[1].ID, result.FinalAssistantMessageID)

	var sawCompleted bool
	for i := 0; i < 6; i++ {
		got := <-events
		require.Equal(t, app.EventAgent, got.Type())
		evt, ok := got.Data().(QueryEvent)
		require.True(t, ok)
		if evt.Status == QueryStatusCompleted {
			sawCompleted = true
			require.Equal(t, "assistant_completed", evt.State.Transition)
			require.Equal(t, "hello", findTextPart(evt.State.Messages[len(evt.State.Messages)-1].Parts))
			break
		}
	}
	require.True(t, sawCompleted)
}

func TestQueryLoopMarksAssistantFailedOnStreamError(t *testing.T) {
	t.Parallel()

	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())
	store := &stubSessionConversationPort{}
	llm := &stubStreamingLLM{
		events: [][]provider.StreamEvent{
			{
				{Type: provider.StreamEventTextDelta, TextDelta: "par"},
				{Type: provider.StreamEventProviderError, Err: errors.New("stream failed")},
			},
		},
	}

	runner := NewQueryLoop(store, llm, dispatcher)

	_, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: "hello",
			},
		},
	})
	require.EqualError(t, err, "stream failed")
	require.Len(t, store.created, 2)
	require.Equal(t, message.KindSystem, store.created[1].Kind)
	require.Contains(t, findTextPart(store.created[1].Parts), "stream failed")

	var sawFailed bool
	for i := 0; i < 6; i++ {
		got := <-events
		evt, ok := got.Data().(QueryEvent)
		require.True(t, ok)
		if evt.Status == QueryStatusFailed {
			sawFailed = true
			require.Equal(t, "stream_failed", evt.State.Transition)
			require.EqualError(t, evt.Err, "stream failed")
			require.Equal(t, "par", findTextPart(evt.State.Messages[len(evt.State.Messages)-1].Parts))
			break
		}
	}
	require.True(t, sawFailed)
}

func TestQueryLoopMapsToolCallStopReasonToAwaitingExecution(t *testing.T) {
	t.Parallel()

	dispatcher := app.NewDispatcher(16)
	store := &stubSessionConversationPort{}
	llm := &stubStreamingLLM{
		events: [][]provider.StreamEvent{
			{
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

	runner := NewQueryLoop(store, llm, dispatcher)

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
	require.NotNil(t, findToolCallPart(store.created[1].Parts))
	require.Equal(t, "call_1", findToolCallPart(store.created[1].Parts).ID)
}

func TestQueryLoopStoresReasoningAndRefusalParts(t *testing.T) {
	t.Parallel()

	store := &stubSessionConversationPort{}
	llm := &stubStreamingLLM{
		events: [][]provider.StreamEvent{
			{
				{Type: provider.StreamEventReasoningDelta, ReasoningDelta: "thinking"},
				{Type: provider.StreamEventRefusalDelta, RefusalDelta: "decline"},
				{Type: provider.StreamEventTurnFinished, StopReason: provider.StopReasonStop},
			},
		},
	}

	runner := NewQueryLoop(store, llm, app.NewDispatcher(16))

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

	require.Len(t, store.created, 2)
	require.NotNil(t, findThinkingPart(store.created[1].Parts))
	require.Equal(t, "decline", findTextPart(store.created[1].Parts))
	require.Equal(t, "thinking", findThinkingPart(store.created[1].Parts).Content)
}

func TestQueryLoopStopsAtConfiguredMaxTurns(t *testing.T) {
	t.Parallel()

	store := &stubSessionConversationPort{}
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

	runner := NewQueryLoop(store, llm, app.NewDispatcher(16))
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
	require.Equal(t, "session-1", llm.calls[1].SessionID)
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

	require.Len(t, state.Messages, 1)
	require.Equal(t, 1, state.TurnCount)
	require.Equal(t, "hello", findTextPart(state.Messages[0].Parts))
}

func TestQueryEventCarriesLoopState(t *testing.T) {
	t.Parallel()

	event := QueryEvent{
		Status: QueryStatusCompleted,
		State: LoopState{
			TurnCount:  2,
			Transition: "assistant_completed",
		},
	}

	require.Equal(t, QueryStatusCompleted, event.Status)
	require.Equal(t, 2, event.State.TurnCount)
}

func TestQueryLoopRunPromptBuildsSingleTextQuery(t *testing.T) {
	t.Parallel()

	store := &stubSessionConversationPort{}
	llm := &stubStreamingLLM{
		events: [][]provider.StreamEvent{
			{
				{Type: provider.StreamEventTurnFinished, StopReason: provider.StopReasonStop},
			},
		},
	}

	runner := NewQueryLoop(store, llm, app.NewDispatcher(16))

	err := runner.RunPrompt(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Len(t, store.created, 2)
	require.Equal(t, "hello", findTextPart(store.created[0].Parts))
}

func TestCopyLoopStateDeepCopiesMessages(t *testing.T) {
	t.Parallel()

	state := LoopState{
		Messages: []message.Message{
			{
				ID:   "assistant-1",
				Kind: message.KindAssistant,
				Parts: []message.Part{
					{
						Type: message.PartTypeText,
						Text: "hello",
					},
					{
						Type: message.PartTypeThinking,
						Thinking: &message.ThinkingPart{
							Content: "plan",
						},
					},
				},
			},
		},
		TurnCount:  2,
		Transition: "assistant_delta_received",
	}

	copied := copyLoopState(state)
	copied.Messages[0].Parts[0].Text = "changed"
	copied.Messages[0].Parts[1].Thinking.Content = "changed-plan"
	copied.Transition = "completed"

	require.Equal(t, "hello", state.Messages[0].Parts[0].Text)
	require.Equal(t, "plan", state.Messages[0].Parts[1].Thinking.Content)
	require.Equal(t, "assistant_delta_received", state.Transition)
	require.Equal(t, "changed", copied.Messages[0].Parts[0].Text)
	require.Equal(t, "changed-plan", copied.Messages[0].Parts[1].Thinking.Content)
	require.Equal(t, "completed", copied.Transition)
}

type stubSessionConversationPort struct {
	created            []message.CreateMessageParams
	hydratedSessionIDs []string
	persisted          []message.Message
}

func (s *stubSessionConversationPort) CreateMessage(_ context.Context, sessionID string, params message.CreateMessageParams, _ app.Dispatcher) (message.Message, error) {
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
		Parts:     nil,
		System:    params.System,
		Progress:  params.Progress,
	}
	if len(params.Parts) > 0 {
		msg.Parts = make([]message.Part, len(params.Parts))
		copy(msg.Parts, params.Parts)
	}
	s.persisted = append(s.persisted, msg)
	return msg, nil
}

func (s *stubSessionConversationPort) ListHistory(_ context.Context, sessionID string, _ app.Dispatcher) ([]message.Message, error) {
	s.hydratedSessionIDs = append(s.hydratedSessionIDs, sessionID)
	if len(s.persisted) == 0 {
		return nil, nil
	}

	copied := make([]message.Message, len(s.persisted))
	for i := range s.persisted {
		copied[i] = s.persisted[i]
		if len(s.persisted[i].Parts) == 0 {
			continue
		}
		copied[i].Parts = make([]message.Part, len(s.persisted[i].Parts))
		copy(copied[i].Parts, s.persisted[i].Parts)
	}
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
		ID:   id,
		Kind: kind,
		Parts: []message.Part{
			{
				Type: message.PartTypeText,
				Text: text,
			},
		},
	}
}

func cloneProviderRequest(req provider.Request) provider.Request {
	return req
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
