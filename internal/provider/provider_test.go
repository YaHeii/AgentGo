package provider

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/YaHeii/agentGo/internal/app"
	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	providercontract "github.com/YaHeii/agentGo/internal/provider/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
)

func TestTurnRunnerContractUsesRequestAndReturnsResult(t *testing.T) {
	t.Parallel()

	var runner TurnRunner = &ProviderService{}
	result, err := runner.RunTurn(context.Background(), providercontract.Request{})

	require.Error(t, err)
	require.Equal(t, providercontract.TurnResult{}, result)
}

func TestNewOpenAIClientRequiresAPIKeyAndModel(t *testing.T) {
	t.Parallel()

	_, err := NewOpenAIClient("", "", "gpt-test")
	require.EqualError(t, err, "openai api key is required")

	_, err = NewOpenAIClient("", "test-key", "")
	require.EqualError(t, err, "openai model is required")
}

func TestRunTurnReturnsErrorWhenMessagesAreEmpty(t *testing.T) {
	t.Parallel()

	svc := NewProviderService(&stubStreamClient{}, nil)

	result, err := svc.RunTurn(context.Background(), providercontract.Request{})
	require.Error(t, err)
	require.Equal(t, providercontract.TurnResult{}, result)
}

func TestRunTurnPassesAgentAssembledRequestToClient(t *testing.T) {
	t.Parallel()

	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())
	client := &stubStreamClient{
		events: []StreamEvent{
			{Type: StreamEventTextDelta, TextDelta: "hi"},
			{Type: StreamEventTurnFinished, StopReason: providercontract.StopReasonStop, SystemFingerprint: "fp-1"},
		},
	}
	svc := NewProviderService(client, dispatcher)

	temperature := float32(0.2)
	maxOutputTokens := 256
	req := providercontract.Request{
		Messages: []messagecontract.Message{
			{
				ID:   "user-1",
				Kind: messagecontract.KindUser,
				Parts: []messagecontract.Part{
					{Type: messagecontract.PartTypeText, Text: "hello"},
				},
			},
		},
		Tools: []toolcontract.Metadata{
			{
				Name:        "grep",
				Description: "search project",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		},
		Context: providercontract.RequestContext{
			Temperature:     &temperature,
			MaxOutputTokens: &maxOutputTokens,
		},
	}

	result, err := svc.RunTurn(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, client.calls, 1)
	require.Equal(t, req, client.calls[0])
	require.Equal(t, "hi", result.Text)
	require.Equal(t, providercontract.StopReasonStop, result.StopReason)
	require.Equal(t, "fp-1", result.SystemFingerprint)

	firstEvent := <-events
	require.Equal(t, app.EventProvider, firstEvent.Type())
	require.Equal(t, app.EventProviderTextDelta, firstEvent.Name())
	require.Equal(t, StreamEventTextDelta, firstEvent.Data().(StreamEvent).Type)

	secondEvent := <-events
	require.Equal(t, app.EventProvider, secondEvent.Type())
	require.Equal(t, app.EventProviderTurnFinished, secondEvent.Name())
	require.Equal(t, StreamEventTurnFinished, secondEvent.Data().(StreamEvent).Type)
}

func TestRunTurnAggregatesStructuredTurnResult(t *testing.T) {
	t.Parallel()

	client := &stubStreamClient{
		events: []StreamEvent{
			{Type: StreamEventTextDelta, TextDelta: "hello "},
			{Type: StreamEventTextDelta, TextDelta: "world"},
			{Type: StreamEventReasoningDelta, ReasoningDelta: "think-1"},
			{Type: StreamEventReasoningDelta, ReasoningDelta: " think-2"},
			{Type: StreamEventRefusalDelta, RefusalDelta: "decline"},
			{Type: StreamEventUsageAvailable, Usage: &providercontract.Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}},
			{Type: StreamEventTurnFinished, StopReason: providercontract.StopReasonStop, SystemFingerprint: "fp-1"},
		},
	}
	svc := NewProviderService(client, nil)

	result, err := svc.RunTurn(context.Background(), providercontract.Request{
		Messages: []messagecontract.Message{userTextMessage("hello")},
	})
	require.NoError(t, err)
	require.Equal(t, "hello world", result.Text)
	require.Equal(t, "think-1 think-2", result.Reasoning)
	require.Equal(t, "decline", result.Refusal)
	require.Equal(t, providercontract.StopReasonStop, result.StopReason)
	require.Equal(t, "fp-1", result.SystemFingerprint)
	require.Equal(t, &providercontract.Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}, result.Usage)
}

func TestRunTurnAggregatesToolCallDeltasIntoResult(t *testing.T) {
	t.Parallel()

	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())
	client := &stubStreamClient{
		events: []StreamEvent{
			{Type: StreamEventToolCallDelta, ToolCallDelta: &ToolCallDelta{Index: 0, ID: "call_1", NameDelta: "gr", ArgumentsDelta: "{\"pat"}},
			{Type: StreamEventToolCallDelta, ToolCallDelta: &ToolCallDelta{Index: 0, NameDelta: "ep", ArgumentsDelta: "tern\":\"go\"}"}},
			{Type: StreamEventTurnFinished, StopReason: providercontract.StopReasonToolCalls},
		},
	}
	svc := NewProviderService(client, dispatcher)

	result, err := svc.RunTurn(context.Background(), providercontract.Request{
		Messages: []messagecontract.Message{userTextMessage("hello")},
	})
	require.NoError(t, err)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, providercontract.ToolCall{
		Index:     0,
		ID:        "call_1",
		Name:      "grep",
		Arguments: "{\"pattern\":\"go\"}",
	}, result.ToolCalls[0])

	firstEvent := <-events
	require.Equal(t, app.EventProviderToolCallDelta, firstEvent.Name())
	require.Equal(t, StreamEventToolCallDelta, firstEvent.Data().(StreamEvent).Type)
	secondEvent := <-events
	require.Equal(t, app.EventProviderToolCallDelta, secondEvent.Name())
	require.Equal(t, StreamEventToolCallDelta, secondEvent.Data().(StreamEvent).Type)
	thirdEvent := <-events
	require.Equal(t, app.EventProviderTurnFinished, thirdEvent.Name())
	require.Equal(t, StreamEventTurnFinished, thirdEvent.Data().(StreamEvent).Type)
}

func TestRunTurnReturnsPartialResultWhenProviderErrorOccurs(t *testing.T) {
	t.Parallel()

	dispatcher := app.NewDispatcher(16)
	events := dispatcher.Subscribe(context.Background())
	client := &stubStreamClient{
		events: []StreamEvent{
			{Type: StreamEventTextDelta, TextDelta: "partial"},
			{Type: StreamEventProviderError, Err: errors.New("stream failed")},
		},
	}
	svc := NewProviderService(client, dispatcher)

	result, err := svc.RunTurn(context.Background(), providercontract.Request{
		Messages: []messagecontract.Message{userTextMessage("hello")},
	})
	require.EqualError(t, err, "stream failed")
	require.Equal(t, "partial", result.Text)

	firstEvent := <-events
	require.Equal(t, app.EventProviderTextDelta, firstEvent.Name())
	require.Equal(t, StreamEventTextDelta, firstEvent.Data().(StreamEvent).Type)
	secondEvent := <-events
	require.Equal(t, app.EventProviderError, secondEvent.Name())
	require.Equal(t, StreamEventProviderError, secondEvent.Data().(StreamEvent).Type)
}

func TestRunTurnReturnsContextErrorAndPartialResultWhenCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	client := &stubStreamClient{
		events: []StreamEvent{
			{Type: StreamEventTextDelta, TextDelta: "partial"},
		},
		afterFirstEvent: cancel,
	}
	svc := NewProviderService(client, nil)

	result, err := svc.RunTurn(ctx, providercontract.Request{
		Messages: []messagecontract.Message{userTextMessage("hello")},
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, "partial", result.Text)
}

func TestStreamEventTypesAreStable(t *testing.T) {
	t.Parallel()

	cases := map[StreamEventType]string{
		StreamEventTextDelta:      "text_delta",
		StreamEventReasoningDelta: "reasoning_delta",
		StreamEventRefusalDelta:   "refusal_delta",
		StreamEventToolCallDelta:  "tool_call_delta",
		StreamEventUsageAvailable: "usage_available",
		StreamEventTurnFinished:   "turn_finished",
		StreamEventProviderError:  "provider_error",
	}

	for got, want := range cases {
		if string(got) != want {
			t.Fatalf("unexpected stream event type: got %q want %q", got, want)
		}
	}
}

func TestBuildChatCompletionRequestMapsMessagesToolsAndContext(t *testing.T) {
	t.Parallel()

	temperature := float32(0.3)
	maxOutputTokens := 128
	req := providercontract.Request{
		Messages: []messagecontract.Message{
			systemTextMessage("system"),
			userTextMessage("user"),
			{
				ID:   "assistant-1",
				Kind: messagecontract.KindAssistant,
				Parts: []messagecontract.Part{
					{Type: messagecontract.PartTypeText, Text: "assistant"},
					{
						Type: messagecontract.PartTypeToolCall,
						ToolCall: &messagecontract.ToolCallPart{
							ID:    "call_1",
							Name:  "grep",
							Input: "{\"pattern\":\"go\"}",
						},
					},
				},
			},
			{
				ID:   "system-1",
				Kind: messagecontract.KindSystem,
				Parts: []messagecontract.Part{
					{Type: messagecontract.PartTypeText, Text: "tool-result"},
					{
						Type: messagecontract.PartTypeToolResult,
						ToolResult: &messagecontract.ToolResultPart{
							ToolCallID: "call_1",
							Content:    "tool-result",
						},
					},
				},
			},
		},
		Tools: []toolcontract.Metadata{
			{
				Name:        "grep",
				Description: "search project",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		},
		Context: providercontract.RequestContext{
			Temperature:     &temperature,
			MaxOutputTokens: &maxOutputTokens,
		},
	}

	got := buildChatCompletionRequest("gpt-test", req)
	require.Equal(t, "gpt-test", got.Model)
	require.True(t, got.Stream)
	require.NotNil(t, got.StreamOptions)
	require.True(t, got.StreamOptions.IncludeUsage)
	require.Equal(t, temperature, got.Temperature)
	require.Len(t, got.Messages, 4)
	require.Equal(t, openai.ChatMessageRoleTool, got.Messages[3].Role)
	require.Equal(t, "call_1", got.Messages[3].ToolCallID)
	require.Len(t, got.Tools, 1)
	require.Equal(t, "grep", got.Tools[0].Function.Name)

	payload, err := json.Marshal(got)
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(payload, &body))
	_, hasMaxCompletionTokens := body["max_completion_tokens"]
	require.False(t, hasMaxCompletionTokens)
	_, hasMaxTokens := body["max_tokens"]
	require.False(t, hasMaxTokens)
}

func TestBuildChatCompletionRequestKeepsToolParametersAsJSONObject(t *testing.T) {
	t.Parallel()

	req := providercontract.Request{
		Messages: []messagecontract.Message{
			userTextMessage("user"),
		},
		Tools: []toolcontract.Metadata{
			{
				Name:        "grep",
				Description: "search project",
				Parameters: json.RawMessage(`{
					"type":"object",
					"properties":{
						"pattern":{"type":"string"}
					},
					"required":["pattern"]
				}`),
			},
		},
	}

	got := buildChatCompletionRequest("gpt-test", req)
	payload, err := json.Marshal(got)
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(payload, &body))

	tools := body["tools"].([]any)
	function := tools[0].(map[string]any)["function"].(map[string]any)
	parameters, ok := function["parameters"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", parameters["type"])
}

func TestPublishChoiceEventsDoesNotEmitToolCallCompleted(t *testing.T) {
	t.Parallel()

	ch := make(chan StreamEvent, 8)
	choice := openai.ChatCompletionStreamChoice{
		Delta: openai.ChatCompletionStreamChoiceDelta{
			ToolCalls: []openai.ToolCall{
				{
					Index: ptr(0),
					ID:    "call_1",
					Function: openai.FunctionCall{
						Name:      "grep",
						Arguments: "{\"pattern\":\"go\"}",
					},
					Type: openai.ToolTypeFunction,
				},
			},
		},
		FinishReason: openai.FinishReasonToolCalls,
	}

	publishChoiceEvents(ch, choice, "fp-1")
	close(ch)

	var events []StreamEvent
	for event := range ch {
		events = append(events, event)
	}

	require.Len(t, events, 2)
	require.Equal(t, StreamEventToolCallDelta, events[0].Type)
	require.Equal(t, StreamEventTurnFinished, events[1].Type)
}

type stubStreamClient struct {
	events          []StreamEvent
	calls           []providercontract.Request
	afterFirstEvent func()
}

func (s *stubStreamClient) Stream(_ context.Context, req providercontract.Request) <-chan StreamEvent {
	s.calls = append(s.calls, req)
	ch := make(chan StreamEvent, len(s.events))
	for i, event := range s.events {
		ch <- event
		if i == 0 && s.afterFirstEvent != nil {
			s.afterFirstEvent()
		}
	}
	close(ch)
	return ch
}

func ptr[T any](v T) *T {
	return &v
}

func userTextMessage(text string) messagecontract.Message {
	return messagecontract.Message{
		ID:   "user",
		Kind: messagecontract.KindUser,
		Parts: []messagecontract.Part{
			{Type: messagecontract.PartTypeText, Text: text},
		},
	}
}

func systemTextMessage(text string) messagecontract.Message {
	return messagecontract.Message{
		ID:   "system",
		Kind: messagecontract.KindSystem,
		Parts: []messagecontract.Part{
			{Type: messagecontract.PartTypeText, Text: text},
		},
	}
}
