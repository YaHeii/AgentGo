package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/YaHeii/agentGo/internal/provider"
	openai "github.com/sashabaranov/go-openai"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

type Client struct {
	client *openai.Client
	model  string
}

func New(cfg Config) (*Client, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("openai api key is required")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, errors.New("openai model is required")
	}

	clientConfig := openai.DefaultConfig(apiKey)
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		clientConfig.BaseURL = baseURL
	}

	return &Client{
		client: openai.NewClientWithConfig(clientConfig),
		model:  model,
	}, nil
}

func (c *Client) Chat(ctx context.Context, messages []provider.Message) (string, error) {
	if len(messages) == 0 {
		return "", errors.New("messages cannot be empty")
	}

	req := openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: make([]openai.ChatCompletionMessage, 0, len(messages)),
	}

	for _, msg := range messages {
		req.Messages = append(req.Messages, openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("create chat completion: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("openai returned no choices")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

func (c *Client) StreamChat(ctx context.Context, req provider.Request) <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent)

	go func() {
		defer close(ch)

		if len(req.Messages) == 0 {
			ch <- provider.StreamEvent{
				Type: provider.StreamEventProviderError,
				Err:  errors.New("messages cannot be empty"),
			}
			return
		}

		openaiReq := openai.ChatCompletionRequest{
			Model:    c.model,
			Stream:   true,
			Messages: make([]openai.ChatCompletionMessage, 0, len(req.Messages)),
			StreamOptions: &openai.StreamOptions{
				IncludeUsage: true,
			},
		}

		for _, msg := range req.Messages {
			openaiReq.Messages = append(openaiReq.Messages, openai.ChatCompletionMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}

		stream, err := c.client.CreateChatCompletionStream(ctx, openaiReq)
		if err != nil {
			ch <- provider.StreamEvent{
				Type: provider.StreamEventProviderError,
				Err:  fmt.Errorf("create chat completion stream: %w", err),
			}
			return
		}
		defer stream.Close()

		toolAccumulator := newToolCallAccumulator()

		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				ch <- provider.StreamEvent{
					Type: provider.StreamEventProviderError,
					Err:  fmt.Errorf("receive stream chunk: %w", err),
				}
				return
			}

			if resp.Usage != nil {
				ch <- provider.StreamEvent{
					Type:              provider.StreamEventUsageAvailable,
					Usage:             toUsage(resp.Usage),
					SystemFingerprint: resp.SystemFingerprint,
				}
			}

			if len(resp.Choices) == 0 {
				continue
			}

			for _, choice := range resp.Choices {
				publishChoiceEvents(ch, choice, resp.SystemFingerprint, toolAccumulator)
			}
		}
	}()

	return ch
}

func (c *Client) Prompt(ctx context.Context, prompt string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("prompt cannot be empty")
	}

	return c.Chat(ctx, []provider.Message{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: prompt,
		},
	})
}

func publishChoiceEvents(
	ch chan<- provider.StreamEvent,
	choice openai.ChatCompletionStreamChoice,
	systemFingerprint string,
	toolAccumulator *toolCallAccumulator,
) {
	if delta := choice.Delta.Content; delta != "" {
		ch <- provider.StreamEvent{
			Type:              provider.StreamEventTextDelta,
			TextDelta:         delta,
			SystemFingerprint: systemFingerprint,
		}
	}

	if delta := choice.Delta.ReasoningContent; delta != "" {
		ch <- provider.StreamEvent{
			Type:              provider.StreamEventReasoningDelta,
			ReasoningDelta:    delta,
			SystemFingerprint: systemFingerprint,
		}
	}

	if delta := choice.Delta.Refusal; delta != "" {
		ch <- provider.StreamEvent{
			Type:              provider.StreamEventRefusalDelta,
			RefusalDelta:      delta,
			SystemFingerprint: systemFingerprint,
		}
	}

	for _, toolCall := range choice.Delta.ToolCalls {
		delta := toToolCallDelta(toolCall)
		toolAccumulator.Apply(delta)
		ch <- provider.StreamEvent{
			Type:              provider.StreamEventToolCallDelta,
			ToolCallDelta:     &delta,
			SystemFingerprint: systemFingerprint,
		}
	}

	stopReason := toStopReason(choice.FinishReason)
	if stopReason == "" {
		return
	}

	if stopReason == provider.StopReasonToolCalls {
		for _, toolCall := range toolAccumulator.Completed() {
			completed := toolCall
			ch <- provider.StreamEvent{
				Type:              provider.StreamEventToolCallCompleted,
				ToolCall:          &completed,
				SystemFingerprint: systemFingerprint,
			}
		}
	}

	ch <- provider.StreamEvent{
		Type:              provider.StreamEventTurnFinished,
		StopReason:        stopReason,
		SystemFingerprint: systemFingerprint,
	}
}

func toUsage(usage *openai.Usage) *provider.Usage {
	if usage == nil {
		return nil
	}

	return &provider.Usage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
}

func toStopReason(reason openai.FinishReason) provider.StopReason {
	switch reason {
	case openai.FinishReasonStop:
		return provider.StopReasonStop
	case openai.FinishReasonLength:
		return provider.StopReasonLength
	case openai.FinishReasonFunctionCall:
		return provider.StopReasonFunctionCall
	case openai.FinishReasonToolCalls:
		return provider.StopReasonToolCalls
	case openai.FinishReasonContentFilter:
		return provider.StopReasonContentFilter
	case "":
		return ""
	case openai.FinishReasonNull:
		return ""
	default:
		return provider.StopReasonUnknown
	}
}

func toToolCallDelta(toolCall openai.ToolCall) provider.ToolCallDelta {
	index := 0
	if toolCall.Index != nil {
		index = *toolCall.Index
	}

	return provider.ToolCallDelta{
		Index:          index,
		ID:             toolCall.ID,
		NameDelta:      toolCall.Function.Name,
		ArgumentsDelta: toolCall.Function.Arguments,
	}
}

type toolCallAccumulator struct {
	calls map[int]provider.ToolCall
	order []int
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{
		calls: make(map[int]provider.ToolCall),
	}
}

func (a *toolCallAccumulator) Apply(delta provider.ToolCallDelta) {
	call, ok := a.calls[delta.Index]
	if !ok {
		call = provider.ToolCall{
			Index: delta.Index,
		}
		a.order = append(a.order, delta.Index)
	}
	if delta.ID != "" {
		call.ID = delta.ID
	}
	if delta.NameDelta != "" {
		call.Name += delta.NameDelta
	}
	if delta.ArgumentsDelta != "" {
		call.Arguments += delta.ArgumentsDelta
	}
	a.calls[delta.Index] = call
}

func (a *toolCallAccumulator) Completed() []provider.ToolCall {
	if len(a.order) == 0 {
		return nil
	}

	out := make([]provider.ToolCall, 0, len(a.order))
	for _, index := range a.order {
		out = append(out, a.calls[index])
	}
	return out
}
