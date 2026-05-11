package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/YaHeii/agentGo/internal/app"
	"github.com/YaHeii/agentGo/internal/message"
	openai "github.com/sashabaranov/go-openai"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

type ProviderService struct {
	messageStore messageStore
	client       streamClient
	dispatcher   app.Dispatcher
}

type Provider struct {
	client *openai.Client
	model  string
}

func NewProviderService(st messageStore, client streamClient, d app.Dispatcher) *ProviderService {
	return &ProviderService{
		messageStore: st,
		client:       client,
		dispatcher:   d,
	}
}

func NewClient(cfg Config) (*Provider, error) {
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

	return &Provider{
		client: openai.NewClientWithConfig(clientConfig),
		model:  model,
	}, nil
}
// Compose Message use req
func (s *ProviderService) StreamChat(ctx context.Context, req Request) <-chan StreamEvent {
	ch := make(chan StreamEvent)

	go func() {
		defer close(ch)

		// The service owns application boundaries: it loads persisted messages,
		// returns the stream to the agent loop, and mirrors each event to app.Dispatcher.
		messages, err := s.messageStore.ListMessages(ctx, req.SessionID, s.dispatcher)
		if err != nil {
			event := StreamEvent{
				Type: StreamEventProviderError,
				Err:  fmt.Errorf("list provider messages: %w", err),
			}
			if s.dispatcher != nil {
				s.dispatcher.Dispatch(app.BaseEvent{
					T:       app.EventProvider,
					Payload: event,
				})
			}
			ch <- event
			return
		}
		// To stream output
		for event := range s.client.streamMessages(ctx, messages) {
			if s.dispatcher != nil {
				s.dispatcher.Dispatch(app.BaseEvent{
					T:       app.EventProvider,
					Payload: event,
				})
			}
			ch <- event
		}
	}()
		//To queryLoop
	return ch
}

func (c *Provider) streamMessages(ctx context.Context, messages []message.Message) <-chan StreamEvent {
	ch := make(chan StreamEvent)

	go func() {
		defer close(ch)

		if len(messages) == 0 {
			ch <- StreamEvent{
				Type: StreamEventProviderError,
				Err:  errors.New("messages cannot be empty"),
			}
			return
		}

		openaiReq := openai.ChatCompletionRequest{
			Model:    c.model,
			Stream:   true,
			Messages: make([]openai.ChatCompletionMessage, 0, len(messages)),
			StreamOptions: &openai.StreamOptions{
				IncludeUsage: true,
			},
		}
		// XXX： send all history message
		for _, msg := range messages {
			role := openai.ChatMessageRoleSystem
			switch msg.Kind {
			case message.KindAssistant:
				role = openai.ChatMessageRoleAssistant
			case message.KindUser:
				role = openai.ChatMessageRoleUser
			}

			content := ""
			for _, part := range msg.Parts {
				if part.Type == message.PartTypeText {
					content = part.Text
					break
				}
			}

			openaiReq.Messages = append(openaiReq.Messages, openai.ChatCompletionMessage{
				Role:    role,
				Content: content,
			})
		}

		stream, err := c.client.CreateChatCompletionStream(ctx, openaiReq)
		if err != nil {
			ch <- StreamEvent{
				Type: StreamEventProviderError,
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

			if len(resp.Choices) == 0 {
				continue
			}

			// The OpenAI client stays dispatcher-agnostic; it only converts wire chunks
			// into provider stream events for the service layer to publish or consume.
			for _, choice := range resp.Choices {
				publishChoiceEvents(ch, choice, resp.SystemFingerprint, toolAccumulator)
			}
		}
	}()

	return ch
}

// publishChoiceEvents maps one OpenAI choice chunk into one or more provider events.
func publishChoiceEvents(
	ch chan<- StreamEvent,
	choice openai.ChatCompletionStreamChoice,
	systemFingerprint string,
	toolAccumulator *toolCallAccumulator,
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
		toolAccumulator.Apply(delta)
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

	if stopReason == StopReasonToolCalls {
		for _, toolCall := range toolAccumulator.Completed() {
			completed := toolCall
			ch <- StreamEvent{
				Type:              StreamEventToolCallCompleted,
				ToolCall:          &completed,
				SystemFingerprint: systemFingerprint,
			}
		}
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
// TODO: seperate to Tool service
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

type toolCallAccumulator struct {
	calls map[int]ToolCall
	order []int
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{
		calls: make(map[int]ToolCall),
	}
}

func (a *toolCallAccumulator) Apply(delta ToolCallDelta) {
	call, ok := a.calls[delta.Index]
	if !ok {
		call = ToolCall{
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

func (a *toolCallAccumulator) Completed() []ToolCall {
	if len(a.order) == 0 {
		return nil
	}

	out := make([]ToolCall, 0, len(a.order))
	for _, index := range a.order {
		out = append(out, a.calls[index])
	}
	return out
}
