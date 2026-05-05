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

func (c *Client) StreamChat(ctx context.Context, messages []provider.Message) <-chan provider.StreamEvent {
	ch := make(chan provider.StreamEvent)

	go func() {
		defer close(ch)

		if len(messages) == 0 {
			ch <- provider.StreamEvent{Err: errors.New("messages cannot be empty")}
			return
		}

		req := openai.ChatCompletionRequest{
			Model:    c.model,
			Stream:   true,
			Messages: make([]openai.ChatCompletionMessage, 0, len(messages)),
		}

		for _, msg := range messages {
			req.Messages = append(req.Messages, openai.ChatCompletionMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}

		stream, err := c.client.CreateChatCompletionStream(ctx, req)
		if err != nil {
			ch <- provider.StreamEvent{Err: fmt.Errorf("create chat completion stream: %w", err)}
			return
		}
		defer stream.Close()

		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				ch <- provider.StreamEvent{Type: provider.StreamEventDone}
				return
			}
			if err != nil {
				ch <- provider.StreamEvent{Err: fmt.Errorf("receive stream chunk: %w", err)}
				return
			}
			if len(resp.Choices) == 0 {
				continue
			}

			delta := resp.Choices[0].Delta.Content
			if delta == "" {
				continue
			}

			ch <- provider.StreamEvent{
				Type:  provider.StreamEventDelta,
				Delta: delta,
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
