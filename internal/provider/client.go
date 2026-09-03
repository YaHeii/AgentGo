package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	providercontract "github.com/YaHeii/agentGo/internal/provider/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	openai "github.com/sashabaranov/go-openai"
)

type OpenAIClient struct {
	client *openai.Client
	model  string
}

func NewOpenAIClient(baseURL string, apiKey string, model string) (*OpenAIClient, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("openai api key is required")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, errors.New("openai model is required")
	}

	clientConfig := openai.DefaultConfig(apiKey)
	if baseURL = strings.TrimSpace(baseURL); baseURL != "" {
		clientConfig.BaseURL = baseURL
	}

	return &OpenAIClient{
		client: openai.NewClientWithConfig(clientConfig),
		model:  model,
	}, nil
}

func (c *OpenAIClient) Stream(ctx context.Context, req providercontract.Request) <-chan StreamEvent {
	ch := make(chan StreamEvent)

	go func() {
		defer close(ch)

		if err := validateRequest(req); err != nil {
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

func buildChatCompletionRequest(model string, req providercontract.Request) openai.ChatCompletionRequest {
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
	if req.Context.MaxOutputTokens != nil && *req.Context.MaxOutputTokens > 0 {
		out.MaxCompletionTokens = *req.Context.MaxOutputTokens
	}

	for _, msg := range req.Messages {
		out.Messages = append(out.Messages, toOpenAIMessage(msg))
	}

	for _, toolDef := range req.Tools {
		out.Tools = append(out.Tools, toolMetadataToOpenAITool(toolDef))
	}

	return out
}

func toOpenAIMessage(msg messagecontract.Message) openai.ChatCompletionMessage {
	switch msg.Kind {
	case messagecontract.KindUser:
		return openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: flattenMessageParts(msg.Parts),
		}
	case messagecontract.KindAssistant:
		out := openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: flattenMessageParts(msg.Parts),
		}
		for _, part := range msg.Parts {
			if part.Type != messagecontract.PartTypeToolCall || part.ToolCall == nil {
				continue
			}
			index := 0
			out.ToolCalls = append(out.ToolCalls, openai.ToolCall{
				Index: &index,
				ID:    part.ToolCall.ID,
				Type:  openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      part.ToolCall.Name,
					Arguments: part.ToolCall.Input,
				},
			})
		}
		return out
	case messagecontract.KindSystem:
		toolResult := firstToolResultPart(msg.Parts)
		if toolResult != nil {
			return openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    flattenMessageParts(msg.Parts),
				ToolCallID: toolResult.ToolCallID,
			}
		}
		return openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: flattenMessageParts(msg.Parts),
		}
	default:
		return openai.ChatCompletionMessage{}
	}
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

func toUsage(usage *openai.Usage) *providercontract.Usage {
	if usage == nil {
		return nil
	}

	return &providercontract.Usage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
}

func toStopReason(reason openai.FinishReason) providercontract.StopReason {
	switch reason {
	case openai.FinishReasonStop:
		return providercontract.StopReasonStop
	case openai.FinishReasonLength:
		return providercontract.StopReasonLength
	case openai.FinishReasonFunctionCall:
		return providercontract.StopReasonFunctionCall
	case openai.FinishReasonToolCalls:
		return providercontract.StopReasonToolCalls
	case openai.FinishReasonContentFilter:
		return providercontract.StopReasonContentFilter
	case "":
		return ""
	case openai.FinishReasonNull:
		return ""
	default:
		return providercontract.StopReasonUnknown
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

func flattenMessageParts(parts []messagecontract.Part) string {
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case messagecontract.PartTypeText:
			if strings.TrimSpace(part.Text) != "" {
				segments = append(segments, part.Text)
			}
		case messagecontract.PartTypeThinking:
			if part.Thinking != nil && strings.TrimSpace(part.Thinking.Content) != "" {
				segments = append(segments, part.Thinking.Content)
			}
		case messagecontract.PartTypeToolResult:
			if part.ToolResult != nil && strings.TrimSpace(part.ToolResult.Content) != "" {
				segments = append(segments, part.ToolResult.Content)
			}
		case messagecontract.PartTypeSummary:
			if strings.TrimSpace(part.Text) != "" {
				segments = append(segments, part.Text)
			}
		}
	}
	return strings.Join(segments, "\n")
}

func firstToolResultPart(parts []messagecontract.Part) *messagecontract.ToolResultPart {
	for _, part := range parts {
		if part.Type == messagecontract.PartTypeToolResult && part.ToolResult != nil {
			return part.ToolResult
		}
	}
	return nil
}

func toolMetadataToOpenAITool(toolDef toolcontract.Metadata) openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        toolDef.Name,
			Description: toolDef.Description,
			Parameters:  toolParameters(toolDef.Parameters),
		},
	}
}

var _ streamClient = (*OpenAIClient)(nil)
