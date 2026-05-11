package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/YaHeii/agentGo/internal/provider"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
)

func TestClientImplementsStreamingLLM(t *testing.T) {
	t.Parallel()

	var _ provider.StreamingLLM = (*Client)(nil)
}

func TestNewRequiresAPIKey(t *testing.T) {
	t.Parallel()

	_, err := New(Config{Model: "gpt-test"})
	if err == nil {
		t.Fatal("expected missing api key error")
	}
	if !strings.Contains(err.Error(), "api key") {
		t.Fatalf("expected api key error, got %v", err)
	}
}

func TestNewRequiresModel(t *testing.T) {
	t.Parallel()

	_, err := New(Config{APIKey: "test-key"})
	if err == nil {
		t.Fatal("expected missing model error")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Fatalf("expected model error, got %v", err)
	}
}

func TestStreamChatRequiresMessages(t *testing.T) {
	t.Parallel()

	client, err := New(Config{
		BaseURL: "http://example.com/v1",
		APIKey:  "test-key",
		Model:   "gpt-test",
	})
	require.NoError(t, err)

	events := collectEvents(client.StreamChat(context.Background(), provider.Request{}))
	require.Len(t, events, 1)
	require.Equal(t, provider.StreamEventProviderError, events[0].Type)
	require.Error(t, events[0].Err)
	require.Contains(t, events[0].Err.Error(), "messages cannot be empty")
}

func TestStreamChatMapsRequestAndStreamEvents(t *testing.T) {
	t.Parallel()

	var requestBody map[string]any

	client := newTestClient(t, "gpt-4o-mini", func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&requestBody))

		body := strings.Join([]string{
			`data: {"id":"1","object":"chat.completion.chunk","created":1729585728,"model":"gpt-4o-mini","system_fingerprint":"fp_test","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
			"",
			`data: {"id":"2","object":"chat.completion.chunk","created":1729585728,"model":"gpt-4o-mini","system_fingerprint":"fp_test","choices":[{"index":0,"delta":{"reasoning_content":"thinking"},"finish_reason":null}]}`,
			"",
			`data: {"id":"3","object":"chat.completion.chunk","created":1729585728,"model":"gpt-4o-mini","system_fingerprint":"fp_test","choices":[{"index":0,"delta":{"refusal":"decline"},"finish_reason":null}]}`,
			"",
			`data: {"id":"4","object":"chat.completion.chunk","created":1729585728,"model":"gpt-4o-mini","system_fingerprint":"fp_test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search","arguments":"{"}}]},"finish_reason":null}]}`,
			"",
			`data: {"id":"5","object":"chat.completion.chunk","created":1729585728,"model":"gpt-4o-mini","system_fingerprint":"fp_test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"type":"function","function":{"arguments":"\"q\":\"golang\"}"}}]},"finish_reason":null}]}`,
			"",
			`data: {"id":"6","object":"chat.completion.chunk","created":1729585728,"model":"gpt-4o-mini","system_fingerprint":"fp_test","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			"",
			`data: {"id":"7","object":"chat.completion.chunk","created":1729585728,"model":"gpt-4o-mini","system_fingerprint":"fp_test","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n")
		return newStreamingResponse(body), nil
	})

	events := collectEvents(client.StreamChat(context.Background(), provider.Request{
		Messages: []provider.Message{
			{
				Role:    "user",
				Content: "hello",
			},
		},
	}))

	require.Len(t, events, 8)

	require.Equal(t, provider.StreamEventTextDelta, events[0].Type)
	require.Equal(t, "hello", events[0].TextDelta)
	require.Equal(t, "fp_test", events[0].SystemFingerprint)

	require.Equal(t, provider.StreamEventReasoningDelta, events[1].Type)
	require.Equal(t, "thinking", events[1].ReasoningDelta)

	require.Equal(t, provider.StreamEventRefusalDelta, events[2].Type)
	require.Equal(t, "decline", events[2].RefusalDelta)

	require.Equal(t, provider.StreamEventToolCallDelta, events[3].Type)
	require.NotNil(t, events[3].ToolCallDelta)
	require.Equal(t, 0, events[3].ToolCallDelta.Index)
	require.Equal(t, "call_1", events[3].ToolCallDelta.ID)
	require.Equal(t, "search", events[3].ToolCallDelta.NameDelta)
	require.Equal(t, "{", events[3].ToolCallDelta.ArgumentsDelta)

	require.Equal(t, provider.StreamEventToolCallDelta, events[4].Type)
	require.NotNil(t, events[4].ToolCallDelta)
	require.Equal(t, "\"q\":\"golang\"}", events[4].ToolCallDelta.ArgumentsDelta)

	require.Equal(t, provider.StreamEventToolCallCompleted, events[5].Type)
	require.NotNil(t, events[5].ToolCall)
	require.Equal(t, "call_1", events[5].ToolCall.ID)
	require.Equal(t, "search", events[5].ToolCall.Name)
	require.Equal(t, "{\"q\":\"golang\"}", events[5].ToolCall.Arguments)

	require.Equal(t, provider.StreamEventTurnFinished, events[6].Type)
	require.Equal(t, provider.StopReasonToolCalls, events[6].StopReason)

	require.Equal(t, provider.StreamEventUsageAvailable, events[7].Type)
	require.NotNil(t, events[7].Usage)
	require.Equal(t, 1, events[7].Usage.PromptTokens)
	require.Equal(t, 2, events[7].Usage.CompletionTokens)
	require.Equal(t, 3, events[7].Usage.TotalTokens)

	streamOptions, ok := requestBody["stream_options"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, streamOptions["include_usage"])

	messages, ok := requestBody["messages"].([]any)
	require.True(t, ok)
	require.Len(t, messages, 1)
	msg, ok := messages[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "user", msg["role"])
	require.Equal(t, "hello", msg["content"])
}

func TestStreamChatEmitsUsageAvailable(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, "gpt-4o-mini", func(_ *http.Request) (*http.Response, error) {
		body := strings.Join([]string{
			`data: {"id":"1","object":"chat.completion.chunk","created":1729585728,"model":"gpt-4o-mini","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n")
		return newStreamingResponse(body), nil
	})

	events := collectEvents(client.StreamChat(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: "user", Content: "hello"}},
	}))

	require.Len(t, events, 1)
	require.Equal(t, provider.StreamEventUsageAvailable, events[0].Type)
	require.NotNil(t, events[0].Usage)
	require.Equal(t, 1, events[0].Usage.PromptTokens)
	require.Equal(t, 2, events[0].Usage.CompletionTokens)
	require.Equal(t, 3, events[0].Usage.TotalTokens)
}

func TestStreamChatEmitsProviderErrorWhenCreateStreamFails(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, "gpt-4o-mini", func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"boom","type":"server_error","param":null,"code":null}}`)),
		}, nil
	})

	events := collectEvents(client.StreamChat(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: "user", Content: "hello"}},
	}))

	require.Len(t, events, 1)
	require.Equal(t, provider.StreamEventProviderError, events[0].Type)
	require.Error(t, events[0].Err)
}

func collectEvents(ch <-chan provider.StreamEvent) []provider.StreamEvent {
	events := make([]provider.StreamEvent, 0)
	for evt := range ch {
		events = append(events, evt)
	}
	return events
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newTestClient(t *testing.T, model string, doer roundTripFunc) *Client {
	t.Helper()

	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = "http://example.com/v1"
	cfg.HTTPClient = doer

	return &Client{
		client: openai.NewClientWithConfig(cfg),
		model:  model,
	}
}

func newStreamingResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
