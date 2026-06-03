package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/YaHeii/agentGo/internal/app"
	message "github.com/YaHeii/agentGo/internal/message/contract"
	"github.com/YaHeii/agentGo/internal/provider"
	providercontract "github.com/YaHeii/agentGo/internal/provider/contract"
)

type testAgentPayload struct {
	State testAgentState
}

type testAgentState struct {
	Messages []message.Message
}

type stubEstimator struct {
	tokens int
	err    error
}

func (s stubEstimator) Estimate(_ string, _ []message.Message) (int, error) {
	return s.tokens, s.err
}

func TestSupervisorRunUpdatesCurrentAndCumulativeUsage(t *testing.T) {
	State = &GlobalState{}
	t.Cleanup(func() { State = nil })
	dispatcher := app.NewDispatcher(16)
	supervisor := NewSupervisor(dispatcher, Config{
		Environment:   "development",
		Model:         "test-model",
		ContextWindow: 400000,
	})
	supervisor.estimator = stubEstimator{tokens: 4}
	if err := supervisor.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize supervisor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		supervisor.Run(ctx)
	}()

	supervisor.handleEvent(app.BaseEvent{
		T: app.EventProvider,
		Payload: provider.StreamEvent{
			Type: provider.StreamEventUsageAvailable,
			Usage: &providercontract.Usage{
				PromptTokens:     11,
				CompletionTokens: 7,
				TotalTokens:      18,
			},
		},
	})
	supervisor.handleEvent(app.BaseEvent{
		T: app.EventProvider,
		Payload: provider.StreamEvent{
			Type: provider.StreamEventUsageAvailable,
			Usage: &providercontract.Usage{
				PromptTokens:     5,
				CompletionTokens: 3,
				TotalTokens:      8,
			},
		},
	})

	waitForCondition(t, func() bool {
		return State.CumulativeTotalTokens == 26 && State.CurrentTurnTotalTokens == 8
	})

	snapshot := *State
	if snapshot.CumulativeInputTokens != 16 {
		t.Fatalf("expected cumulative input tokens 16, got %d", snapshot.CumulativeInputTokens)
	}
	if snapshot.CumulativeOutputTokens != 10 {
		t.Fatalf("expected cumulative output tokens 10, got %d", snapshot.CumulativeOutputTokens)
	}
	if snapshot.CurrentTurnInputTokens != 5 {
		t.Fatalf("expected current turn input 5, got %d", snapshot.CurrentTurnInputTokens)
	}
	if snapshot.CurrentTurnOutputTokens != 3 {
		t.Fatalf("expected current turn output 3, got %d", snapshot.CurrentTurnOutputTokens)
	}
	if snapshot.ActualContextTokens != 5 {
		t.Fatalf("expected actual context tokens 5, got %d", snapshot.ActualContextTokens)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor run did not exit after context cancellation")
	}
}

func TestSupervisorRunEstimatesContextFromAgentEvents(t *testing.T) {
	State = &GlobalState{}
	t.Cleanup(func() { State = nil })
	dispatcher := app.NewDispatcher(16)
	supervisor := NewSupervisor(dispatcher, Config{
		Environment:   "development",
		Model:         "test-model",
		ContextWindow: 400000,
	})
	supervisor.estimator = stubEstimator{tokens: 4}
	if err := supervisor.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize supervisor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		supervisor.Run(ctx)
	}()

	supervisor.handleEvent(app.BaseEvent{
		T: app.EventAgent,
		Payload: testAgentPayload{
			State: testAgentState{
				Messages: []message.Message{
					{
						Kind: message.KindUser,
						Parts: []message.Part{
							{Type: message.PartTypeText, Text: "hello"},
						},
					},
					{
						Kind: message.KindAssistant,
						Parts: []message.Part{
							{
								Type: message.PartTypeThinking,
								Thinking: &message.ThinkingPart{
									Content: "plan",
								},
							},
							{Type: message.PartTypeText, Text: "world"},
						},
					},
				},
			},
		},
	})

	waitForCondition(t, func() bool {
		return State.EstimatedContextTokens == 4 && State.CurrentMessageCount == 2
	})

	snapshot := *State
	if snapshot.EstimatedContextTokens != 4 {
		t.Fatalf("expected estimated context tokens 4, got %d", snapshot.EstimatedContextTokens)
	}
	if snapshot.EstimatedContextChars <= 0 {
		t.Fatalf("expected estimated context chars > 0, got %d", snapshot.EstimatedContextChars)
	}
	if snapshot.CurrentMessageCount != 2 {
		t.Fatalf("expected message count 2, got %d", snapshot.CurrentMessageCount)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor run did not exit after context cancellation")
	}
}

func TestSupervisorEstimateFallsBackWhenModelEncodingUnknown(t *testing.T) {
	State = &GlobalState{}
	t.Cleanup(func() { State = nil })
	dispatcher := app.NewDispatcher(16)
	supervisor := NewSupervisor(dispatcher, Config{
		Environment:   "development",
		Model:         "unknown-model",
		ContextWindow: 400000,
	})
	supervisor.estimator = stubEstimator{err: assertiveError("estimation failed")}
	if err := supervisor.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize supervisor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		supervisor.Run(ctx)
	}()

	supervisor.handleEvent(app.BaseEvent{
		T: app.EventAgent,
		Payload: testAgentPayload{
			State: testAgentState{
				Messages: []message.Message{
					{
						Kind: message.KindUser,
						Parts: []message.Part{
							{Type: message.PartTypeText, Text: "fallback"},
						},
					},
				},
			},
		},
	})

	waitForCondition(t, func() bool {
		return State.EstimatedContextChars > 0 && State.CurrentMessageCount == 1
	})

	snapshot := *State
	if snapshot.EstimatedContextChars != len("fallback") {
		t.Fatalf("expected fallback char count %d, got %d", len("fallback"), snapshot.EstimatedContextChars)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor run did not exit after context cancellation")
	}
}

func TestExtractMessagesFromAgentPayload(t *testing.T) {
	payload := testAgentPayload{
		State: testAgentState{
			Messages: []message.Message{
				{
					Kind: message.KindUser,
					Parts: []message.Part{
						{Type: message.PartTypeText, Text: "hello"},
					},
				},
			},
		},
	}

	messages, ok := extractMessagesFromAgentPayload(payload)
	if !ok {
		t.Fatal("expected payload extraction to succeed")
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Parts[0].Text != "hello" {
		t.Fatalf("expected extracted text hello, got %q", messages[0].Parts[0].Text)
	}
}

func waitForCondition(t *testing.T, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

type assertiveError string

func (e assertiveError) Error() string {
	return string(e)
}
