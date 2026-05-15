package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

type OpenAIClient struct {
	client *openai.Client
	model  string
}

func NewOpenAIClient(cfg Config) (*OpenAIClient, error) {
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

	return &OpenAIClient{
		client: openai.NewClientWithConfig(clientConfig),
		model:  model,
	}, nil
}

func (c *OpenAIClient) Stream(ctx context.Context, req Request) <-chan StreamEvent {
	ch := make(chan StreamEvent)

	go func() {
		defer close(ch)

		if err := req.Validate(); err != nil {
			ch <- StreamEvent{
				Type: StreamEventProviderError,
				Err:  err,
			}
			return
		}

		openaiReq := buildChatCompletionRequest(c.model, req)
		stream, err := c.client.CreateChatCompletionStream(ctx, openaiReq)
		if err != nil {
			ch <- StreamEvent{
				Type: StreamEventProviderError,
				Err:  fmt.Errorf("create chat completion stream: %w", err),
			}
			return
		}
		defer stream.Close()

		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				ch <- StreamEvent{
					Type: StreamEventProviderError,
					Err:  fmt.Errorf("receive stream chunk: %w", err),
				}
				return
			}

			if resp.Usage != nil {
				ch <- StreamEvent{
					Type:              StreamEventUsageAvailable,
					Usage:             toUsage(resp.Usage),
					SystemFingerprint: resp.SystemFingerprint,
				}
			}

			for _, choice := range resp.Choices {
				publishChoiceEvents(ch, choice, resp.SystemFingerprint)
			}
		}
	}()

	return ch
}

func buildChatCompletionRequest(model string, req Request) openai.ChatCompletionRequest {
	out := openai.ChatCompletionRequest{
		Model:    model,
		Stream:   true,
		Messages: make([]openai.ChatCompletionMessage, 0, len(req.Messages)),
		Tools:    make([]openai.Tool, 0, len(req.Tools)),
		StreamOptions: &openai.StreamOptions{
			IncludeUsage: true,
		},
	}

	if req.Context.Temperature != nil {
		out.Temperature = *req.Context.Temperature
	}

	for _, msg := range req.Messages {
		out.Messages = append(out.Messages, toOpenAIMessage(msg))
	}

	for _, toolDef := range req.Tools {
		out.Tools = append(out.Tools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        toolDef.Name,
				Description: toolDef.Description,
				Parameters:  toolParameters(toolDef.Parameters),
			},
		})
	}

	return out
}

func toOpenAIMessage(msg Message) openai.ChatCompletionMessage {
	out := openai.ChatCompletionMessage{
		Role:       string(msg.Role),
		Content:    msg.Content,
		ToolCallID: msg.ToolCallID,
	}

	if len(msg.ToolCalls) > 0 {
		out.ToolCalls = make([]openai.ToolCall, 0, len(msg.ToolCalls))
		for i := range msg.ToolCalls {
			toolCall := msg.ToolCalls[i]
			index := toolCall.Index
			out.ToolCalls = append(out.ToolCalls, openai.ToolCall{
				Index: &index,
				ID:    toolCall.ID,
				Type:  openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      toolCall.Name,
					Arguments: toolCall.Arguments,
				},
			})
		}
	}

	return out
}

func publishChoiceEvents(
	ch chan<- StreamEvent,
	choice openai.ChatCompletionStreamChoice,
	systemFingerprint string,
) {
	if delta := choice.Delta.Content; delta != "" {
		ch <- StreamEvent{
			Type:              StreamEventTextDelta,
			TextDelta:         delta,
			SystemFingerprint: systemFingerprint,
		}
	}

	if delta := choice.Delta.ReasoningContent; delta != "" {
		ch <- StreamEvent{
			Type:              StreamEventReasoningDelta,
			ReasoningDelta:    delta,
			SystemFingerprint: systemFingerprint,
		}
	}

	if delta := choice.Delta.Refusal; delta != "" {
		ch <- StreamEvent{
			Type:              StreamEventRefusalDelta,
			RefusalDelta:      delta,
			SystemFingerprint: systemFingerprint,
		}
	}

	for _, toolCall := range choice.Delta.ToolCalls {
		delta := toToolCallDelta(toolCall)
		ch <- StreamEvent{
			Type:              StreamEventToolCallDelta,
			ToolCallDelta:     &delta,
			SystemFingerprint: systemFingerprint,
		}
	}

	stopReason := toStopReason(choice.FinishReason)
	if stopReason == "" {
		return
	}

	ch <- StreamEvent{
		Type:              StreamEventTurnFinished,
		StopReason:        stopReason,
		SystemFingerprint: systemFingerprint,
	}
}

func toUsage(usage *openai.Usage) *Usage {
	if usage == nil {
		return nil
	}

	return &Usage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
}

func toStopReason(reason openai.FinishReason) StopReason {
	switch reason {
	case openai.FinishReasonStop:
		return StopReasonStop
	case openai.FinishReasonLength:
		return StopReasonLength
	case openai.FinishReasonFunctionCall:
		return StopReasonFunctionCall
	case openai.FinishReasonToolCalls:
		return StopReasonToolCalls
	case openai.FinishReasonContentFilter:
		return StopReasonContentFilter
	case "":
		return ""
	case openai.FinishReasonNull:
		return ""
	default:
		return StopReasonUnknown
	}
}

func toToolCallDelta(toolCall openai.ToolCall) ToolCallDelta {
	index := 0
	if toolCall.Index != nil {
		index = *toolCall.Index
	}

	return ToolCallDelta{
		Index:          index,
		ID:             toolCall.ID,
		NameDelta:      toolCall.Function.Name,
		ArgumentsDelta: toolCall.Function.Arguments,
	}
}

func toolParameters(raw []byte) any {
	if len(raw) == 0 {
		return map[string]any{}
	}

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return map[string]any{}
	}
	return schema
}

var _ streamClient = (*OpenAIClient)(nil)
