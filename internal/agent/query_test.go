package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	agentcontract "github.com/YaHeii/agentGo/internal/agent/contract"
	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	"github.com/YaHeii/agentGo/internal/provider"
	"github.com/YaHeii/agentGo/internal/tool"
	"github.com/stretchr/testify/require"
)

func TestNewQueryLoopReturnsRunner(t *testing.T) {
	runner := NewQueryLoop(&stubAgentApp{}, &stubProvider{}, app.NewDispatcher(16))
	require.NotNil(t, runner)
}

func TestNewQueryLoopSeedsConfigAndDeps(t *testing.T) {
	appSvc := &stubAgentApp{}
	providerSvc := &stubProvider{}
	dispatcher := app.NewDispatcher(16)

	runner := NewQueryLoop(appSvc, providerSvc, dispatcher)

	require.Equal(t, 10, runner.config.MaxTurns)
	require.Same(t, appSvc, runner.deps.App)
	require.Same(t, providerSvc, runner.deps.Provider)
	require.Same(t, dispatcher, runner.deps.dispatcher)
}

func TestRenderLoopstateBuildsProviderRequestFromLoopStateAndRuntimeState(t *testing.T) {
	appSvc := &stubAgentApp{
		metas: []tool.Metadata{
			{
				Name:          "grep",
				Description:   "search files",
				Parameters:    json.RawMessage(`{"type":"object","required":["pattern"]}`),
				Enabled:       true,
				SecurityLevel: tool.SafeLevel,
			},
		},
	}
	runner := NewQueryLoop(appSvc, &stubProvider{}, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:      "v0.1.0",
			ProjectRoot:     "/workspace/project",
			Cwd:             "/workspace/project/internal",
			PermissionLevel: tool.SafeLevel,
			Temperature:     0.2,
			ModelLimit:      4096,
		},
	})
	req, err := runner.renderLoopstate(LoopState{
		Messages: []message.Message{
			messageRecord("assistant-1", message.KindAssistant, "previous answer"),
			messageRecord("user-1", message.KindUser, "find tests"),
			{
				ID:        "assistant-pending",
				SessionID: "session-1",
				Kind:      message.KindAssistant,
				Parts: []message.Part{
					{Type: message.PartTypeText, Text: ""},
				},
			},
		},
		PendingToolCalls: []provider.ToolCall{
			{
				ID:        "call-1",
				Name:      "grep",
				Arguments: `{"pattern":"*.go"}`,
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, req.Messages, 2)
	require.Equal(t, provider.RoleAssistant, req.Messages[0].Role)
	require.Equal(t, "previous answer", req.Messages[0].Content)
	require.Equal(t, provider.RoleUser, req.Messages[1].Role)
	require.Equal(t, "find tests", req.Messages[1].Content)
	require.Len(t, req.Tools, 1)
	require.Equal(t, "grep", req.Tools[0].Name)
	require.JSONEq(t, `{"type":"object","required":["pattern"]}`, string(req.Tools[0].Parameters))
	require.NotNil(t, req.Context.Temperature)
	require.NotNil(t, req.Context.MaxOutputTokens)
	require.Equal(t, float32(0.2), *req.Context.Temperature)
	require.Equal(t, 4096, *req.Context.MaxOutputTokens)
}

func TestRenderPromptBuildsSystemPromptFromRuntimeState(t *testing.T) {
	appSvc := &stubAgentApp{
		metas: []tool.Metadata{
			{
				Name:          "grep",
				Description:   "search files",
				Parameters:    json.RawMessage(`{"type":"object","required":["pattern"]}`),
				Enabled:       true,
				SecurityLevel: tool.SafeLevel,
			},
		},
	}
	runner := NewQueryLoop(appSvc, &stubProvider{}, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:      "v0.1.0",
			ProjectRoot:     "/workspace/project",
			Cwd:             "/workspace/project/internal",
			PermissionLevel: tool.SafeLevel,
		},
	})
	prompt, err := runner.renderPrompt(LoopState{
		Messages: []message.Message{
			messageRecord("assistant-1", message.KindAssistant, "previous answer"),
			messageRecord("user-1", message.KindUser, "find tests"),
			{
				ID:        "assistant-pending",
				SessionID: "session-1",
				Kind:      message.KindAssistant,
				Parts: []message.Part{
					{Type: message.PartTypeText, Text: ""},
				},
			},
		},
	}, "find tests")
	require.NoError(t, err)
	require.Contains(t, prompt, "/workspace/project")
	require.Contains(t, prompt, "/workspace/project/internal")
	require.Contains(t, prompt, "find tests")
	require.Contains(t, prompt, "grep")
	require.Contains(t, prompt, "search files")
	require.NotContains(t, prompt, "assistant-pending")
	require.Equal(t, []tool.SecurityLevel{tool.SafeLevel}, appSvc.listToolCalls)
}

func TestBuildInitialRequestInjectsInitPromptOnce(t *testing.T) {
	appSvc := &stubAgentApp{
		metas: []tool.Metadata{
			{
				Name:          "grep",
				Description:   "search files",
				Parameters:    json.RawMessage(`{"type":"object","required":["pattern"]}`),
				Enabled:       true,
				SecurityLevel: tool.SafeLevel,
			},
		},
	}
	runner := NewQueryLoop(appSvc, &stubProvider{}, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:      "v0.1.0",
			ProjectRoot:     "/workspace/project",
			Cwd:             "/workspace/project/internal",
			PermissionLevel: tool.SafeLevel,
		},
	})

	req, err := runner.buildInitialRequest(LoopState{
		Messages: []message.Message{
			messageRecord("user-1", message.KindUser, "find tests"),
		},
	}, "full init prompt")
	require.NoError(t, err)
	require.Len(t, req.Messages, 2)
	require.Equal(t, provider.RoleSystem, req.Messages[0].Role)
	require.Equal(t, "full init prompt", req.Messages[0].Content)
	require.Equal(t, provider.RoleUser, req.Messages[1].Role)
	require.Equal(t, "find tests", req.Messages[1].Content)
}

func TestQueryLoopRunQueryUsesInjectedDeps(t *testing.T) {
	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())

	originalApp := &stubAgentApp{}
	originalProvider := &stubProvider{}

	depsApp := &stubAgentApp{}
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
	depsApp.metas = []tool.Metadata{{
		Name:        "grep",
		Description: "search files",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Enabled:     true,
	}}

	runner := NewQueryLoop(originalApp, originalProvider, dispatcher)
	runner.deps.App = depsApp
	runner.deps.Provider = depsProvider
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:  "v0.1.0",
			ProjectRoot: "/workspace/project",
			Cwd:         "/workspace/project",
		},
	})

	_, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Len(t, originalApp.created, 0)
	require.Len(t, depsApp.created, 2)
	require.Len(t, originalProvider.calls, 0)
	require.Len(t, depsProvider.calls, 1)
	require.NotEmpty(t, depsProvider.calls[0].Messages)
	require.Equal(t, provider.RoleSystem, depsProvider.calls[0].Messages[0].Role)

	gotStarted := <-events
	require.Equal(t, app.EventAgent, gotStarted.Type())
	started, ok := gotStarted.Data().(QueryEvent)
	require.True(t, ok)
	require.Equal(t, QueryStatusStarted, started.Status)
	require.Equal(t, 1, countSystemMessages(depsProvider.calls[0].Messages))
}

func TestQueryLoopRejectsNonPositiveMaxTurns(t *testing.T) {
	runner := NewQueryLoop(&stubAgentApp{}, &stubProvider{}, app.NewDispatcher(16))
	runner.config.MaxTurns = 0

	_, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.EqualError(t, err, "agent: max turns must be greater than 0")
}

func TestQueryLoopAssemblesProviderRequestWithSystemHistoryAndTools(t *testing.T) {
	appSvc := &stubAgentApp{
		persisted: []message.Message{
			messageRecord("assistant-old", message.KindAssistant, "previous answer"),
		},
		metas: []tool.Metadata{
			{
				Name:          "grep",
				Description:   "search files",
				Parameters:    json.RawMessage(`{"type":"object","required":["pattern"]}`),
				Enabled:       true,
				SecurityLevel: tool.SafeLevel,
			},
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
	runner := NewQueryLoop(appSvc, providerSvc, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:  "v0.1.0",
			ProjectRoot: "/workspace/project",
			Cwd:         "/workspace/project/subdir",
		},
	})

	_, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Len(t, providerSvc.calls, 1)

	call := providerSvc.calls[0]
	require.Len(t, call.Messages, 3)
	require.Equal(t, provider.RoleSystem, call.Messages[0].Role)
	require.Contains(t, call.Messages[0].Content, "# Role & Authority")
	require.Equal(t, provider.RoleAssistant, call.Messages[1].Role)
	require.Equal(t, "previous answer", call.Messages[1].Content)
	require.Equal(t, provider.RoleUser, call.Messages[2].Role)
	require.Equal(t, "hello", call.Messages[2].Content)
	require.Len(t, call.Tools, 1)
	require.Equal(t, "grep", call.Tools[0].Name)
	require.JSONEq(t, `{"type":"object","required":["pattern"]}`, string(call.Tools[0].Parameters))
}

func TestQueryLoopCreatesMessagesAndPersistsAssistantReply(t *testing.T) {
	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())
	appSvc := &stubAgentApp{}
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

	runner := NewQueryLoop(appSvc, providerSvc, dispatcher)
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:  "v0.1.0",
			ProjectRoot: "/workspace/project",
			Cwd:         "/workspace/project",
		},
	})

	result, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Equal(t, "session-1", result.SessionID)
	require.Equal(t, "user-1", result.UserMessageID)
	require.Equal(t, 1, result.Turns)
	require.Equal(t, FinishReasonCompleted, result.FinishReason)
	require.Len(t, appSvc.created, 2)
	require.Equal(t, []string{"session-1"}, appSvc.hydratedSessionIDs)
	require.Equal(t, "hello", findTextPart(appSvc.created[1].Parts))
	require.Equal(t, appSvc.persisted[1].ID, result.FinalAssistantMessageID)

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
	appSvc := &stubAgentApp{}
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

	runner := NewQueryLoop(appSvc, providerSvc, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:  "v0.1.0",
			ProjectRoot: "/workspace/project",
			Cwd:         "/workspace/project",
		},
	})

	_, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.NoError(t, err)

	require.Len(t, appSvc.created, 2)
	require.NotNil(t, findThinkingPart(appSvc.created[1].Parts))
	require.Equal(t, "decline", findTextPart(appSvc.created[1].Parts))
	require.Equal(t, "thinking", findThinkingPart(appSvc.created[1].Parts).Content)
}

func TestQueryLoopMarksAssistantFailedOnProviderError(t *testing.T) {
	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())
	appSvc := &stubAgentApp{}
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

	runner := NewQueryLoop(appSvc, providerSvc, dispatcher)
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:  "v0.1.0",
			ProjectRoot: "/workspace/project",
			Cwd:         "/workspace/project",
		},
	})

	_, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.EqualError(t, err, "stream failed")
	require.Len(t, appSvc.created, 3)
	require.Equal(t, message.KindSystem, appSvc.created[2].Kind)
	require.Contains(t, findTextPart(appSvc.created[2].Parts), "stream failed")

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
	appSvc := &stubAgentApp{}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				err: context.Canceled,
			},
		},
	}

	runner := NewQueryLoop(appSvc, providerSvc, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:  "v0.1.0",
			ProjectRoot: "/workspace/project",
			Cwd:         "/workspace/project",
		},
	})

	_, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.ErrorIs(t, err, context.Canceled)
}

func TestQueryLoopRunsToolLoopAndFeedsToolResultsBackToProvider(t *testing.T) {
	appSvc := &stubAgentApp{}
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
	appSvc.metas = []tool.Metadata{{
		Name:              "grep",
		Description:       "search files",
		Parameters:        json.RawMessage(`{"type":"object","required":["pattern"]}`),
		Enabled:           true,
		IsConcurrencySafe: true,
	}}
	appSvc.callResults = []tool.ToolResult{{
		ToolCallID: "call_1",
		Name:       "grep",
		Status:     tool.StatusSuccess,
		Content:    `{"matches":[{"path":"main.go"}]}`,
	}}

	runner := NewQueryLoop(appSvc, providerSvc, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:      "v0.1.0",
			PermissionLevel: tool.AttentionLevel,
			ProjectRoot:     "/workspace/project",
			Cwd:             "/workspace/project/work",
		},
	})

	result, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Equal(t, FinishReasonCompleted, result.FinishReason)
	require.GreaterOrEqual(t, len(providerSvc.calls), 1)
	require.Len(t, appSvc.callBatches, 1)
	require.Len(t, appSvc.callBatches[0].Calls, 1)
	require.Equal(t, "call_1", appSvc.callBatches[0].Calls[0].ToolCallID)
	require.Equal(t, "grep", appSvc.callBatches[0].Calls[0].Name)
	require.Equal(t, "session-1", appSvc.callBatches[0].Calls[0].Context.SessionID)
	require.Equal(t, "turn-1", appSvc.callBatches[0].Calls[0].Context.TurnID)
	require.Equal(t, "/workspace/project", appSvc.callBatches[0].Calls[0].Context.WorkspaceRoot)
	require.Equal(t, "/workspace/project/work", appSvc.callBatches[0].Calls[0].Context.WorkingDir)
	require.Equal(t, tool.AttentionLevel, appSvc.callBatches[0].Calls[0].PermissionLevel)
	require.Len(t, appSvc.created, 4)
	require.NotNil(t, findToolCallPart(appSvc.created[1].Parts))
	require.Equal(t, "call_1", findToolCallPart(appSvc.created[1].Parts).ID)
	require.NotNil(t, findToolResultPart(appSvc.created[2].Parts))
	require.Equal(t, "call_1", findToolResultPart(appSvc.created[2].Parts).ToolCallID)
	require.Equal(t, `{"matches":[{"path":"main.go"}]}`, findTextPart(appSvc.created[2].Parts))
	require.Equal(t, "tool answered", findTextPart(appSvc.created[3].Parts))
	require.True(t, requestContainsToolResult(providerSvc.calls[len(providerSvc.calls)-1], "call_1"))
	if len(providerSvc.calls) > 1 {
		require.Equal(t, 0, countSystemMessages(providerSvc.calls[1].Messages))
	}
}

func TestQueryLoopPreservesToolResultBusinessErrorsAndContinues(t *testing.T) {
	appSvc := &stubAgentApp{}
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
	appSvc.metas = []tool.Metadata{{
		Name:        "grep",
		Description: "search files",
		Parameters:  json.RawMessage(`{"type":"object","required":["pattern"]}`),
		Enabled:     true,
	}}
	appSvc.callResults = []tool.ToolResult{{
		ToolCallID: "call_1",
		Name:       "grep",
		Status:     tool.StatusExecutionError,
		Content:    "path escaped workspace",
		Err:        errors.New("path escaped workspace"),
	}}

	runner := NewQueryLoop(appSvc, providerSvc, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:  "v0.1.0",
			ProjectRoot: "/workspace/project",
			Cwd:         "/workspace/project",
		},
	})

	result, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Equal(t, FinishReasonCompleted, result.FinishReason)
	require.GreaterOrEqual(t, len(providerSvc.calls), 1)
	require.NotNil(t, findToolResultPart(appSvc.created[2].Parts))
	require.True(t, findToolResultPart(appSvc.created[2].Parts).IsError)
	if len(providerSvc.calls) > 1 {
		require.Equal(t, 0, countSystemMessages(providerSvc.calls[1].Messages))
	}
}

func TestQueryLoopFailsWhenToolBatchValidationFails(t *testing.T) {
	appSvc := &stubAgentApp{}
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
	appSvc.metas = []tool.Metadata{{
		Name:        "grep",
		Description: "search files",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Enabled:     true,
	}}
	appSvc.callErr = errors.New("tool batch failed")

	runner := NewQueryLoop(appSvc, providerSvc, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:  "v0.1.0",
			ProjectRoot: "/workspace/project",
			Cwd:         "/workspace/project",
		},
	})

	_, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.EqualError(t, err, "tool batch failed")
}

func TestQueryLoopStopsAtConfiguredMaxTurns(t *testing.T) {
	appSvc := &stubAgentApp{}
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

	runner := NewQueryLoop(appSvc, providerSvc, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:  "v0.1.0",
			ProjectRoot: "/workspace/project",
			Cwd:         "/workspace/project",
		},
	})
	runner.config.MaxTurns = 2

	result, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Len(t, providerSvc.calls, 2)
	require.Equal(t, 2, result.Turns)
	require.Equal(t, FinishReasonCompleted, result.FinishReason)
}

func TestNewLoopStateSeedsMessagesAndTurnCount(t *testing.T) {
	state := newLoopState("session-1", []message.Part{
		{Type: message.PartTypeText, Text: "hello"},
	})

	require.Len(t, state.Messages, 1)
	require.Equal(t, 1, state.TurnCount)
	require.Equal(t, "hello", findTextPart(state.Messages[0].Parts))
}

func TestQueryLoopMicroCompactsHistoryWhenEstimatedTokensExceedModelLimit(t *testing.T) {
	appSvc := &stubAgentApp{
		persisted: []message.Message{
			messageRecord("assistant-old", message.KindAssistant, "previous answer"),
		},
	}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				result: provider.TurnResult{
					StopReason: provider.StopReasonStop,
				},
			},
		},
	}

	runner := NewQueryLoop(appSvc, providerSvc, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:  "v0.1.0",
			Model:       "gpt-4o-mini",
			ModelLimit:  1,
			ProjectRoot: "/workspace/project",
			Cwd:         "/workspace/project",
		},
	})

	_, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Len(t, providerSvc.calls, 1)
	require.Len(t, providerSvc.calls[0].Messages, 2)
	require.Equal(t, provider.RoleSystem, providerSvc.calls[0].Messages[0].Role)
	require.Equal(t, provider.RoleUser, providerSvc.calls[0].Messages[1].Role)
	require.Equal(t, "hello", providerSvc.calls[0].Messages[1].Content)
}

func TestQueryLoopUsesMessageWindowBeforeModelLimitCompaction(t *testing.T) {
	appSvc := &stubAgentApp{
		persisted: []message.Message{
			messageRecord("user-old", message.KindUser, "old user"),
			messageRecord("assistant-old", message.KindAssistant, "old assistant"),
			messageRecord("user-mid", message.KindUser, "mid user"),
			messageRecord("assistant-mid", message.KindAssistant, "mid assistant"),
		},
	}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				result: provider.TurnResult{
					StopReason: provider.StopReasonStop,
				},
			},
		},
	}

	runner := NewQueryLoop(appSvc, providerSvc, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:  "v0.1.0",
			Model:       "gpt-4o-mini",
			ModelLimit:  1000,
			ProjectRoot: "/workspace/project",
			Cwd:         "/workspace/project",
		},
	})
	runner.config.MessageWindow = 2

	_, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Len(t, providerSvc.calls, 1)
	require.Len(t, providerSvc.calls[0].Messages, 3)
	require.Equal(t, provider.RoleSystem, providerSvc.calls[0].Messages[0].Role)
	require.Equal(t, provider.RoleAssistant, providerSvc.calls[0].Messages[1].Role)
	require.Equal(t, "mid assistant", providerSvc.calls[0].Messages[1].Content)
	require.Equal(t, provider.RoleUser, providerSvc.calls[0].Messages[2].Role)
	require.Equal(t, "hello", providerSvc.calls[0].Messages[2].Content)
}

func TestQueryLoopOnlyInjectsInitPromptOnFirstTurn(t *testing.T) {
	appSvc := &stubAgentApp{}
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
					Text:       "two",
					StopReason: provider.StopReasonStop,
				},
			},
		},
	}

	runner := NewQueryLoop(appSvc, providerSvc, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:  "v0.1.0",
			ProjectRoot: "/workspace/project",
			Cwd:         "/workspace/project",
		},
	})
	runner.config.MaxTurns = 2

	_, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Len(t, providerSvc.calls, 2)
	require.Equal(t, 1, countSystemMessages(providerSvc.calls[0].Messages))
	require.Equal(t, 0, countSystemMessages(providerSvc.calls[1].Messages))
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

func TestQueryLoopRunQueryBuildsSingleTextQuery(t *testing.T) {
	appSvc := &stubAgentApp{}
	providerSvc := &stubProvider{
		results: []stubTurn{
			{
				result: provider.TurnResult{
					StopReason: provider.StopReasonStop,
				},
			},
		},
	}

	runner := NewQueryLoop(appSvc, providerSvc, app.NewDispatcher(16))
	runner.SetRuntimeProvider(stubRuntimeProvider{
		snapshot: agentcontract.RuntimeSnapshot{
			AppVersion:  "v0.1.0",
			ProjectRoot: "/workspace/project",
			Cwd:         "/workspace/project",
		},
	})

	_, err := runner.RunQuery(context.Background(), "session-1", "hello")
	require.NoError(t, err)
	require.Len(t, appSvc.created, 2)
	require.Equal(t, "hello", findTextPart(appSvc.created[0].Parts))
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

type stubAgentApp struct {
	created            []message.CreateMessageParams
	hydratedSessionIDs []string
	persisted          []message.Message
	metas              []tool.Metadata
	listToolCalls      []tool.SecurityLevel
	callBatches        []tool.BatchRequest
	callResults        []tool.ToolResult
	callErr            error
}

func (s *stubAgentApp) CreateMessage(_ context.Context, params message.CreateMessageParams) (message.Message, error) {
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
		SessionID: params.SessionID,
		Kind:      params.Kind,
		Parts:     cloneMessagePartsForTest(params.Parts),
		System:    params.System,
		Progress:  params.Progress,
	}
	s.persisted = append(s.persisted, msg)
	return msg, nil
}

func (s *stubAgentApp) ListHistory(_ context.Context, sessionID string) ([]message.Message, error) {
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

func (s *stubAgentApp) ListTools(_ context.Context, permissionLevel tool.SecurityLevel) []tool.Metadata {
	s.listToolCalls = append(s.listToolCalls, permissionLevel)
	out := make([]tool.Metadata, len(s.metas))
	copy(out, s.metas)
	return out
}

func (s *stubAgentApp) CallTools(_ context.Context, req tool.BatchRequest) ([]tool.ToolResult, error) {
	s.callBatches = append(s.callBatches, cloneBatchRequest(req))
	if s.callErr != nil {
		return nil, s.callErr
	}
	out := make([]tool.ToolResult, len(s.callResults))
	copy(out, s.callResults)
	return out, nil
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

func countSystemMessages(messages []provider.Message) int {
	count := 0
	for _, msg := range messages {
		if msg.Role == provider.RoleSystem {
			count++
		}
	}
	return count
}

func requestContainsToolResult(req provider.Request, toolCallID string) bool {
	for _, msg := range req.Messages {
		if msg.Role == provider.RoleTool && msg.ToolCallID == toolCallID {
			return true
		}
	}
	return false
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

type stubRuntimeProvider struct {
	snapshot  agentcontract.RuntimeSnapshot
	tokenizer agentcontract.TokenEncoder
	err       error
}

func (s stubRuntimeProvider) Snapshot() agentcontract.RuntimeSnapshot {
	return s.snapshot
}

func (s stubRuntimeProvider) TokenizerForModel(_ string) (agentcontract.TokenEncoder, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.tokenizer != nil {
		return s.tokenizer, nil
	}
	return stubTokenEncoder{}, nil
}

type stubTokenEncoder struct{}

func (stubTokenEncoder) Encode(text string, _ []string, _ []string) []int {
	if text == "" {
		return nil
	}
	return make([]int, len(text))
}
