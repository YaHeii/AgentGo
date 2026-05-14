package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/lifecycle"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/tool"
	"github.com/stretchr/testify/require"
)

func TestNewQueryLoopReturnsRunner(t *testing.T) {
	runner := NewQueryLoop(&stubSessionConversationPort{}, &stubProvider{}, app.NewDispatcher(16))
	require.NotNil(t, runner)
}

func TestNewQueryLoopSeedsConfigAndDeps(t *testing.T) {
	store := &stubSessionConversationPort{}
	providerSvc := &stubProvider{}
	dispatcher := app.NewDispatcher(16)

	runner := NewQueryLoop(store, providerSvc, dispatcher)

	require.Equal(t, 10, runner.config.MaxTurns)
	require.Same(t, store, runner.deps.Conversation)
	require.Same(t, providerSvc, runner.deps.Provider)
	require.Same(t, dispatcher, runner.deps.dispatcher)
}

func TestRenderPromptUsesLifecycleStateToolMetadataAndUserInput(t *testing.T) {
	resetGlobalStateForAgentTest()
	lifecycle.State = &lifecycle.GlobalState{
		AppVersion:  "v0.1.0",
		ProjectRoot: "/workspace/project",
		Cwd:         "/workspace/project/internal",
	}

	runner := NewQueryLoop(&stubSessionConversationPort{}, &stubProvider{}, app.NewDispatcher(16))
	runner.deps.Tools = &stubToolRunner{
		metas: []tool.Metadata{
			{
				Name:        "grep",
				Description: "search files",
				Parameters:  json.RawMessage(`{"type":"object","required":["pattern"]}`),
			},
		},
	}

	rendered, err := runner.renderPrompt(PromptContext{
		AppVersion:  "v0.1.0",
		ProjectRoot: "/workspace/project",
		Cwd:         "/workspace/project/internal",
		UserInput:   "find tests",
		Tools: []tool.Metadata{
			{
				Name:        "grep",
				Description: "search files",
				Parameters:  json.RawMessage(`{"type":"object","required":["pattern"]}`),
			},
		},
	})
	require.NoError(t, err)
	require.Contains(t, rendered, "/workspace/project")
	require.Contains(t, rendered, "/workspace/project/internal")
	require.Contains(t, rendered, "find tests")
	require.Contains(t, rendered, "grep")
	require.Contains(t, rendered, "search files")
}

func TestQueryLoopRunQueryUsesInjectedDeps(t *testing.T) {
	resetGlobalStateForAgentTest()
	lifecycle.State = &lifecycle.GlobalState{
		AppVersion:  "v0.1.0",
		ProjectRoot: "/workspace/project",
		Cwd:         "/workspace/project",
	}

	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())

	originalStore := &stubSessionConversationPort{}
	originalProvider := &stubProvider{}

	depsStore := &stubSessionConversationPort{}
	depsProvider := &stubProvider{
		results: []stubTurn{
			{
				result: provider.TurnResult{
					Text:       "hi",
					StopReason: provider.StopReasonStop,
				},
			},
		},
	}
	depsTools := &stubToolRunner{
		metas: []tool.Metadata{
			{
				Name:        "grep",
				Description: "search files",
				Parameters:  json.RawMessage(`{"type":"object"}`),
				Enabled:     true,
			},
		},
	}

	runner := NewQueryLoop(originalStore, originalProvider, dispatcher)
	runner.deps.Conversation = depsStore
	runner.deps.Provider = depsProvider
	runner.deps.Tools = depsTools

	_, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{Type: message.PartTypeText, Text: "hello"},
		},
	})
	require.NoError(t, err)
	require.Len(t, originalStore.created, 0)
	require.Len(t, depsStore.created, 2)
	require.Len(t, originalProvider.calls, 0)
	require.Len(t, depsProvider.calls, 1)
	require.Len(t, depsTools.listCalls, 1)
	require.NotEmpty(t, depsProvider.calls[0].Messages)
	require.Equal(t, provider.RoleSystem, depsProvider.calls[0].Messages[0].Role)

	gotStarted := <-events
	require.Equal(t, app.EventAgent, gotStarted.Type())
	started, ok := gotStarted.Data().(QueryEvent)
	require.True(t, ok)
	require.Equal(t, QueryStatusStarted, started.Status)
}

func TestQueryLoopRejectsNonPositiveMaxTurns(t *testing.T) {
	runner := NewQueryLoop(&stubSessionConversationPort{}, &stubProvider{}, app.NewDispatcher(16))
	runner.config.MaxTurns = 0

	_, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{Type: message.PartTypeText, Text: "hello"},
		},
	})
	require.EqualError(t, err, "agent: max turns must be greater than 0")
}

func TestQueryLoopAssemblesProviderRequestWithSystemHistoryAndTools(t *testing.T) {
	resetGlobalStateForAgentTest()
	lifecycle.State = &lifecycle.GlobalState{
		AppVersion:  "v0.1.0",
		ProjectRoot: "/workspace/project",
		Cwd:         "/workspace/project/subdir",
	}

	store := &stubSessionConversationPort{
		persisted: []message.Message{
			messageRecord("assistant-old", message.KindAssistant, "previous answer"),
		},
	}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				result: provider.TurnResult{
					Text:       "new answer",
					StopReason: provider.StopReasonStop,
				},
			},
		},
	}
	tools := &stubToolRunner{
		metas: []tool.Metadata{
			{
				Name:              "grep",
				Description:       "search files",
				Parameters:        json.RawMessage(`{"type":"object","required":["pattern"]}`),
				Enabled:           true,
				SecurityLevel:     tool.SafeLevel,
				IsConcurrencySafe: true,
			},
		},
	}

	runner := NewQueryLoop(store, providerSvc, app.NewDispatcher(16))
	runner.deps.Tools = tools

	_, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{Type: message.PartTypeText, Text: "hello"},
		},
	})
	require.NoError(t, err)
	require.Len(t, providerSvc.calls, 1)

	call := providerSvc.calls[0]
	require.Len(t, call.Messages, 3)
	require.Equal(t, provider.RoleSystem, call.Messages[0].Role)
	require.Equal(t, provider.RoleAssistant, call.Messages[1].Role)
	require.Equal(t, "previous answer", call.Messages[1].Content)
	require.Equal(t, provider.RoleUser, call.Messages[2].Role)
	require.Equal(t, "hello", call.Messages[2].Content)
	require.Len(t, call.Tools, 1)
	require.Equal(t, "grep", call.Tools[0].Name)
	require.JSONEq(t, `{"type":"object","required":["pattern"]}`, string(call.Tools[0].Parameters))
}

func TestQueryLoopCreatesMessagesAndPersistsAssistantReply(t *testing.T) {
	resetGlobalStateForAgentTest()
	lifecycle.State = &lifecycle.GlobalState{
		AppVersion:  "v0.1.0",
		ProjectRoot: "/workspace/project",
		Cwd:         "/workspace/project",
	}

	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())
	store := &stubSessionConversationPort{}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				result: provider.TurnResult{
					Text:       "hello",
					StopReason: provider.StopReasonStop,
				},
			},
		},
	}

	runner := NewQueryLoop(store, providerSvc, dispatcher)
	runner.deps.Tools = &stubToolRunner{}

	result, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{Type: message.PartTypeText, Text: "hello"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "session-1", result.SessionID)
	require.Equal(t, "user-1", result.UserMessageID)
	require.Equal(t, 1, result.Turns)
	require.Equal(t, FinishReasonCompleted, result.FinishReason)
	require.Len(t, store.created, 2)
	require.Equal(t, []string{"session-1"}, store.hydratedSessionIDs)
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

func TestQueryLoopStoresReasoningAndRefusalParts(t *testing.T) {
	resetGlobalStateForAgentTest()
	lifecycle.State = &lifecycle.GlobalState{
		AppVersion:  "v0.1.0",
		ProjectRoot: "/workspace/project",
		Cwd:         "/workspace/project",
	}

	store := &stubSessionConversationPort{}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				result: provider.TurnResult{
					Reasoning:  "thinking",
					Refusal:    "decline",
					StopReason: provider.StopReasonStop,
				},
			},
		},
	}

	runner := NewQueryLoop(store, providerSvc, app.NewDispatcher(16))
	runner.deps.Tools = &stubToolRunner{}

	_, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{Type: message.PartTypeText, Text: "hello"},
		},
	})
	require.NoError(t, err)

	require.Len(t, store.created, 2)
	require.NotNil(t, findThinkingPart(store.created[1].Parts))
	require.Equal(t, "decline", findTextPart(store.created[1].Parts))
	require.Equal(t, "thinking", findThinkingPart(store.created[1].Parts).Content)
}

func TestQueryLoopMarksAssistantFailedOnProviderError(t *testing.T) {
	resetGlobalStateForAgentTest()
	lifecycle.State = &lifecycle.GlobalState{
		AppVersion:  "v0.1.0",
		ProjectRoot: "/workspace/project",
		Cwd:         "/workspace/project",
	}

	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())
	store := &stubSessionConversationPort{}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				result: provider.TurnResult{
					Text: "par",
				},
				err: errors.New("stream failed"),
			},
		},
	}

	runner := NewQueryLoop(store, providerSvc, dispatcher)
	runner.deps.Tools = &stubToolRunner{}

	_, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{Type: message.PartTypeText, Text: "hello"},
		},
	})
	require.EqualError(t, err, "stream failed")
	require.Len(t, store.created, 3)
	require.Equal(t, message.KindSystem, store.created[2].Kind)
	require.Contains(t, findTextPart(store.created[2].Parts), "stream failed")

	var sawFailed bool
	for i := 0; i < 8; i++ {
		got := <-events
		evt, ok := got.Data().(QueryEvent)
		require.True(t, ok)
		if evt.Status == QueryStatusFailed {
			sawFailed = true
			require.Equal(t, "provider_error_recorded", evt.State.Transition)
			require.EqualError(t, evt.Err, "stream failed")
			require.Equal(t, "par", findTextPart(evt.State.Messages[len(evt.State.Messages)-2].Parts))
			break
		}
	}
	require.True(t, sawFailed)
}

func TestQueryLoopMarksCancelledOnContextCancellation(t *testing.T) {
	resetGlobalStateForAgentTest()
	lifecycle.State = &lifecycle.GlobalState{
		AppVersion:  "v0.1.0",
		ProjectRoot: "/workspace/project",
		Cwd:         "/workspace/project",
	}

	store := &stubSessionConversationPort{}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				err: context.Canceled,
			},
		},
	}

	runner := NewQueryLoop(store, providerSvc, app.NewDispatcher(16))
	runner.deps.Tools = &stubToolRunner{}

	_, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{Type: message.PartTypeText, Text: "hello"},
		},
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestQueryLoopRunsToolLoopAndFeedsToolResultsBackToProvider(t *testing.T) {
	resetGlobalStateForAgentTest()
	lifecycle.State = &lifecycle.GlobalState{
		AppVersion:  "v0.1.0",
		ProjectRoot: "/workspace/project",
		Cwd:         "/workspace/project/work",
		SessionID:   "runtime-session",
	}

	store := &stubSessionConversationPort{}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				result: provider.TurnResult{
					ToolCalls: []provider.ToolCall{
						{
							Index:     0,
							ID:        "call_1",
							Name:      "grep",
							Arguments: `{"pattern":"go"}`,
						},
					},
					StopReason: provider.StopReasonToolCalls,
				},
			},
			{
				result: provider.TurnResult{
					Text:       "tool answered",
					StopReason: provider.StopReasonStop,
				},
			},
		},
	}
	toolSvc := &stubToolRunner{
		metas: []tool.Metadata{
			{
				Name:              "grep",
				Description:       "search files",
				Parameters:        json.RawMessage(`{"type":"object","required":["pattern"]}`),
				Enabled:           true,
				IsConcurrencySafe: true,
			},
		},
		callResults: []tool.ToolResult{
			{
				ToolCallID: "call_1",
				Name:       "grep",
				Status:     tool.StatusSuccess,
				Content:    `{"matches":[{"path":"main.go"}]}`,
			},
		},
	}

	runner := NewQueryLoop(store, providerSvc, app.NewDispatcher(16))
	runner.deps.Tools = toolSvc

	result, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{Type: message.PartTypeText, Text: "hello"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, FinishReasonCompleted, result.FinishReason)
	require.Len(t, providerSvc.calls, 2)
	require.Len(t, toolSvc.callBatches, 1)
	require.Len(t, toolSvc.callBatches[0].Calls, 1)
	require.Equal(t, "call_1", toolSvc.callBatches[0].Calls[0].ToolCallID)
	require.Equal(t, "grep", toolSvc.callBatches[0].Calls[0].Name)
	require.Equal(t, "session-1", toolSvc.callBatches[0].Calls[0].Context.SessionID)
	require.Equal(t, "turn-1", toolSvc.callBatches[0].Calls[0].Context.TurnID)
	require.Equal(t, "/workspace/project", toolSvc.callBatches[0].Calls[0].Context.WorkspaceRoot)
	require.Equal(t, "/workspace/project/work", toolSvc.callBatches[0].Calls[0].Context.WorkingDir)
	require.Len(t, store.created, 4)
	require.NotNil(t, findToolCallPart(store.created[1].Parts))
	require.Equal(t, "call_1", findToolCallPart(store.created[1].Parts).ID)
	require.NotNil(t, findToolResultPart(store.created[2].Parts))
	require.Equal(t, "call_1", findToolResultPart(store.created[2].Parts).ToolCallID)
	require.Equal(t, `{"matches":[{"path":"main.go"}]}`, findTextPart(store.created[2].Parts))
	require.Equal(t, provider.RoleTool, providerSvc.calls[1].Messages[len(providerSvc.calls[1].Messages)-1].Role)
	require.Equal(t, "call_1", providerSvc.calls[1].Messages[len(providerSvc.calls[1].Messages)-1].ToolCallID)
}

func TestQueryLoopPreservesToolResultBusinessErrorsAndContinues(t *testing.T) {
	resetGlobalStateForAgentTest()
	lifecycle.State = &lifecycle.GlobalState{
		AppVersion:  "v0.1.0",
		ProjectRoot: "/workspace/project",
		Cwd:         "/workspace/project",
	}

	store := &stubSessionConversationPort{}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				result: provider.TurnResult{
					ToolCalls: []provider.ToolCall{
						{
							Index:     0,
							ID:        "call_1",
							Name:      "grep",
							Arguments: `{"pattern":"go"}`,
						},
					},
					StopReason: provider.StopReasonToolCalls,
				},
			},
			{
				result: provider.TurnResult{
					Text:       "handled error",
					StopReason: provider.StopReasonStop,
				},
			},
		},
	}
	toolSvc := &stubToolRunner{
		metas: []tool.Metadata{
			{
				Name:        "grep",
				Description: "search files",
				Parameters:  json.RawMessage(`{"type":"object","required":["pattern"]}`),
				Enabled:     true,
			},
		},
		callResults: []tool.ToolResult{
			{
				ToolCallID: "call_1",
				Name:       "grep",
				Status:     tool.StatusExecutionError,
				Content:    "path escaped workspace",
				Err:        errors.New("path escaped workspace"),
			},
		},
	}

	runner := NewQueryLoop(store, providerSvc, app.NewDispatcher(16))
	runner.deps.Tools = toolSvc

	result, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{Type: message.PartTypeText, Text: "hello"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, FinishReasonCompleted, result.FinishReason)
	require.Len(t, providerSvc.calls, 2)
	require.NotNil(t, findToolResultPart(store.created[2].Parts))
	require.True(t, findToolResultPart(store.created[2].Parts).IsError)
}

func TestQueryLoopFailsWhenToolBatchValidationFails(t *testing.T) {
	resetGlobalStateForAgentTest()
	lifecycle.State = &lifecycle.GlobalState{
		AppVersion:  "v0.1.0",
		ProjectRoot: "/workspace/project",
		Cwd:         "/workspace/project",
	}

	store := &stubSessionConversationPort{}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				result: provider.TurnResult{
					ToolCalls: []provider.ToolCall{
						{
							Index:     0,
							ID:        "call_1",
							Name:      "grep",
							Arguments: `{"pattern":"go"}`,
						},
					},
					StopReason: provider.StopReasonToolCalls,
				},
			},
		},
	}
	toolSvc := &stubToolRunner{
		metas: []tool.Metadata{
			{
				Name:        "grep",
				Description: "search files",
				Parameters:  json.RawMessage(`{"type":"object"}`),
				Enabled:     true,
			},
		},
		callErr: errors.New("tool batch failed"),
	}

	runner := NewQueryLoop(store, providerSvc, app.NewDispatcher(16))
	runner.deps.Tools = toolSvc

	_, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{Type: message.PartTypeText, Text: "hello"},
		},
	})
	require.EqualError(t, err, "tool batch failed")
}

func TestQueryLoopStopsAtConfiguredMaxTurns(t *testing.T) {
	resetGlobalStateForAgentTest()
	lifecycle.State = &lifecycle.GlobalState{
		AppVersion:  "v0.1.0",
		ProjectRoot: "/workspace/project",
		Cwd:         "/workspace/project",
	}

	store := &stubSessionConversationPort{}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				result: provider.TurnResult{
					Text:       "one",
					StopReason: provider.StopReasonLength,
				},
			},
			{
				result: provider.TurnResult{
					StopReason: provider.StopReasonStop,
				},
			},
		},
	}

	runner := NewQueryLoop(store, providerSvc, app.NewDispatcher(16))
	runner.deps.Tools = &stubToolRunner{}
	runner.config.MaxTurns = 2

	result, err := runner.RunQuery(context.Background(), QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{Type: message.PartTypeText, Text: "hello"},
		},
	})
	require.NoError(t, err)
	require.Len(t, providerSvc.calls, 2)
	require.Equal(t, 2, result.Turns)
	require.Equal(t, FinishReasonCompleted, result.FinishReason)
}

func TestNewLoopStateSeedsMessagesAndTurnCount(t *testing.T) {
	state := newLoopState(QueryParams{
		SessionID: "session-1",
		InputParts: []message.Part{
			{Type: message.PartTypeText, Text: "hello"},
		},
	})

	require.Len(t, state.Messages, 1)
	require.Equal(t, 1, state.TurnCount)
	require.Equal(t, "hello", findTextPart(state.Messages[0].Parts))
}

func TestQueryEventCarriesLoopState(t *testing.T) {
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
	resetGlobalStateForAgentTest()
	lifecycle.State = &lifecycle.GlobalState{
		AppVersion:  "v0.1.0",
		ProjectRoot: "/workspace/project",
		Cwd:         "/workspace/project",
	}

	store := &stubSessionConversationPort{}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				result: provider.TurnResult{
					StopReason: provider.StopReasonStop,
				},
			},
		},
	}

	runner := NewQueryLoop(store, providerSvc, app.NewDispatcher(16))
	runner.deps.Tools = &stubToolRunner{}

	err := runner.RunPrompt(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Len(t, store.created, 2)
	require.Equal(t, "hello", findTextPart(store.created[0].Parts))
}

func TestCopyLoopStateDeepCopiesMessages(t *testing.T) {
	state := LoopState{
		Messages: []message.Message{
			{
				ID:   "assistant-1",
				Kind: message.KindAssistant,
				Parts: []message.Part{
					{Type: message.PartTypeText, Text: "hello"},
					{
						Type: message.PartTypeThinking,
						Thinking: &message.ThinkingPart{
							Content: "plan",
						},
					},
					{
						Type: message.PartTypeToolResult,
						ToolResult: &message.ToolResultPart{
							ToolCallID: "call-1",
							Content:    "ok",
							IsError:    true,
						},
					},
				},
			},
		},
		TurnCount:          2,
		Transition:         "assistant_delta_received",
		AssistantMessageID: "assistant-1",
		FinishReason:       FinishReasonAwaitingToolExecution,
		StopReason:         provider.StopReasonToolCalls,
		PendingToolCalls: []provider.ToolCall{
			{
				ID:        "call-1",
				Name:      "search",
				Arguments: "{\"q\":\"go\"}",
			},
		},
	}

	copied := copyLoopState(state)
	copied.Messages[0].Parts[0].Text = "changed"
	copied.Messages[0].Parts[1].Thinking.Content = "changed-plan"
	copied.Messages[0].Parts[2].ToolResult.Content = "changed-result"
	copied.PendingToolCalls[0].Arguments = "{\"q\":\"changed\"}"
	copied.Transition = "completed"

	require.Equal(t, "hello", state.Messages[0].Parts[0].Text)
	require.Equal(t, "plan", state.Messages[0].Parts[1].Thinking.Content)
	require.Equal(t, "ok", state.Messages[0].Parts[2].ToolResult.Content)
	require.Equal(t, "{\"q\":\"go\"}", state.PendingToolCalls[0].Arguments)
	require.Equal(t, "assistant_delta_received", state.Transition)
	require.Equal(t, "assistant-1", state.AssistantMessageID)
	require.Equal(t, FinishReasonAwaitingToolExecution, state.FinishReason)
	require.Equal(t, provider.StopReasonToolCalls, state.StopReason)
	require.Equal(t, "changed", copied.Messages[0].Parts[0].Text)
	require.Equal(t, "changed-plan", copied.Messages[0].Parts[1].Thinking.Content)
	require.Equal(t, "changed-result", copied.Messages[0].Parts[2].ToolResult.Content)
	require.Equal(t, "{\"q\":\"changed\"}", copied.PendingToolCalls[0].Arguments)
	require.Equal(t, "completed", copied.Transition)
}

type stubTurn struct {
	result provider.TurnResult
	err    error
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
		Parts:     cloneMessagePartsForTest(params.Parts),
		System:    params.System,
		Progress:  params.Progress,
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
		copied[i].Parts = cloneMessagePartsForTest(s.persisted[i].Parts)
	}
	return copied, nil
}

type stubProvider struct {
	results []stubTurn
	calls   []provider.Request
}

func (s *stubProvider) RunTurn(_ context.Context, req provider.Request) (provider.TurnResult, error) {
	s.calls = append(s.calls, cloneProviderRequest(req))
	index := len(s.calls) - 1
	if index >= len(s.results) {
		return provider.TurnResult{}, nil
	}
	return cloneTurnResult(s.results[index].result), s.results[index].err
}

type stubToolRunner struct {
	metas       []tool.Metadata
	listCalls   []struct{}
	callBatches []tool.BatchRequest
	callResults []tool.ToolResult
	callErr     error
}

func (s *stubToolRunner) ListTools(_ context.Context) []tool.Metadata {
	s.listCalls = append(s.listCalls, struct{}{})
	out := make([]tool.Metadata, len(s.metas))
	copy(out, s.metas)
	return out
}

func (s *stubToolRunner) Call(_ context.Context, req tool.BatchRequest) ([]tool.ToolResult, error) {
	s.callBatches = append(s.callBatches, cloneBatchRequest(req))
	if s.callErr != nil {
		return nil, s.callErr
	}
	out := make([]tool.ToolResult, len(s.callResults))
	copy(out, s.callResults)
	return out, nil
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
	out := provider.Request{
		Context: req.Context,
	}
	if len(req.Messages) > 0 {
		out.Messages = make([]provider.Message, len(req.Messages))
		for i := range req.Messages {
			out.Messages[i] = req.Messages[i]
			if len(req.Messages[i].ToolCalls) > 0 {
				out.Messages[i].ToolCalls = append([]provider.ToolCall(nil), req.Messages[i].ToolCalls...)
			}
		}
	}
	if len(req.Tools) > 0 {
		out.Tools = make([]provider.ToolDefinition, len(req.Tools))
		copy(out.Tools, req.Tools)
	}
	return out
}

func cloneTurnResult(result provider.TurnResult) provider.TurnResult {
	out := result
	if len(result.ToolCalls) > 0 {
		out.ToolCalls = append([]provider.ToolCall(nil), result.ToolCalls...)
	}
	if result.Usage != nil {
		usage := *result.Usage
		out.Usage = &usage
	}
	return out
}

func cloneBatchRequest(req tool.BatchRequest) tool.BatchRequest {
	out := tool.BatchRequest{}
	if len(req.Calls) > 0 {
		out.Calls = make([]tool.ToolCallRequest, len(req.Calls))
		copy(out.Calls, req.Calls)
	}
	return out
}

func cloneMessagePartsForTest(parts []message.Part) []message.Part {
	if len(parts) == 0 {
		return nil
	}

	copied := make([]message.Part, len(parts))
	for i := range parts {
		copied[i] = parts[i]
		if parts[i].ToolCall != nil {
			toolCall := *parts[i].ToolCall
			copied[i].ToolCall = &toolCall
		}
		if parts[i].ToolResult != nil {
			toolResult := *parts[i].ToolResult
			copied[i].ToolResult = &toolResult
		}
		if parts[i].Thinking != nil {
			thinking := *parts[i].Thinking
			copied[i].Thinking = &thinking
		}
	}
	return copied
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

func findToolResultPart(parts []message.Part) *message.ToolResultPart {
	for _, part := range parts {
		if part.Type == message.PartTypeToolResult && part.ToolResult != nil {
			return part.ToolResult
		}
	}
	return nil
}

func resetGlobalStateForAgentTest() {
	lifecycle.State = nil
}
