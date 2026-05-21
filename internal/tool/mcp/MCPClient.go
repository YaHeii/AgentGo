package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

type TransportKind string

const (
	TransportStdio          TransportKind = "stdio"
	TransportStreamableHTTP TransportKind = "streamable_http"
)

type Config struct {
	Name    string
	Kind    TransportKind
	Command string
	Args    []string
	Env     []string
	URL     string
	Headers map[string]string
}

type Client interface {
	Start(ctx context.Context) error
	ListTools(ctx context.Context) ([]toolcontract.Metadata, error)
	CallTool(ctx context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult
	Close() error
}

type MCPClient struct {
	name    string
	kind    TransportKind
	client  *mcpclient.Client
	session mcpSession
}

type mcpSession interface {
	Start(ctx context.Context) error
	Initialize(ctx context.Context, request mcpsdk.InitializeRequest) (*mcpsdk.InitializeResult, error)
	ListTools(ctx context.Context, request mcpsdk.ListToolsRequest) (*mcpsdk.ListToolsResult, error)
	CallTool(ctx context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error)
	Close() error
}

func NewClient(cfg Config) (*MCPClient, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	trans, err := buildTransport(cfg)
	if err != nil {
		return nil, err
	}
	client := &MCPClient{
		name: strings.TrimSpace(cfg.Name),
		kind: cfg.Kind,
	}
	client.client = mcpclient.NewClient(trans)
	client.session = client.client
	return client, nil
}

func (c *MCPClient) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.session.Start(ctx); err != nil {
		return err
	}
	_, err := c.session.Initialize(ctx, mcpsdk.InitializeRequest{
		Params: mcpsdk.InitializeParams{
			ProtocolVersion: mcpsdk.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcpsdk.Implementation{
				Name:  "agentGo",
				Title: c.name,
			},
		},
	})
	return err
}

func (c *MCPClient) ListTools(ctx context.Context) ([]toolcontract.Metadata, error) {
	result, err := c.session.ListTools(ctx, mcpsdk.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	tools := make([]toolcontract.Metadata, 0, len(result.Tools))
	for _, tool := range result.Tools {
		meta, err := c.mapToolMetadata(tool)
		if err != nil {
			return nil, err
		}
		tools = append(tools, meta)
	}
	return tools, nil
}

func (c *MCPClient) CallTool(ctx context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
	arguments, err := decodeArguments(req.Arguments)
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusValidationFailed,
			Content:    "failed to decode mcp tool arguments",
			Err:        err,
		}
	}

	result, err := c.session.CallTool(ctx, mcpsdk.CallToolRequest{
		Params: mcpsdk.CallToolParams{
			Name:      req.Name,
			Arguments: arguments,
		},
	})
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    "mcp tool call failed",
			Err:        err,
		}
	}
	return mapToolResult(req, result)
}

func (c *MCPClient) Close() error {
	return c.session.Close()
}

func (c *MCPClient) mapToolMetadata(tool mcpsdk.Tool) (toolcontract.Metadata, error) {
	parameters, err := toolParameters(tool)
	if err != nil {
		return toolcontract.Metadata{}, err
	}

	requirements := toolcontract.RequireNone
	if c.kind == TransportStreamableHTTP {
		requirements = toolcontract.RequireNetwork
	}

	return toolcontract.Metadata{
		Name:              tool.Name,
		Description:       tool.Description,
		Parameters:        parameters,
		Enabled:           true,
		SecurityLevel:     toolcontract.AttentionLevel,
		IsConcurrencySafe: false,
		Requirements:      requirements,
	}, nil
}

func validateConfig(cfg Config) error {
	switch cfg.Kind {
	case TransportStdio:
		if strings.TrimSpace(cfg.Command) == "" {
			return fmt.Errorf("stdio command is required")
		}
	case TransportStreamableHTTP:
		if strings.TrimSpace(cfg.URL) == "" {
			return fmt.Errorf("streamable_http url is required")
		}
	default:
		return fmt.Errorf("unsupported mcp transport %q", cfg.Kind)
	}
	return nil
}

func buildTransport(cfg Config) (transport.Interface, error) {
	switch cfg.Kind {
	case TransportStdio:
		return transport.NewStdioWithOptions(strings.TrimSpace(cfg.Command), cfg.Env, cfg.Args), nil
	case TransportStreamableHTTP:
		opts := make([]transport.StreamableHTTPCOption, 0, 1)
		if len(cfg.Headers) > 0 {
			opts = append(opts, transport.WithHTTPHeaders(cfg.Headers))
		}
		return transport.NewStreamableHTTP(strings.TrimSpace(cfg.URL), opts...)
	default:
		return nil, fmt.Errorf("unsupported mcp transport %q", cfg.Kind)
	}
}

func toolParameters(tool mcpsdk.Tool) (json.RawMessage, error) {
	if len(tool.RawInputSchema) > 0 {
		return cloneRawMessage(tool.RawInputSchema), nil
	}
	data, err := json.Marshal(tool.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("marshal mcp tool %q input schema: %w", tool.Name, err)
	}
	return json.RawMessage(data), nil
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}

func decodeArguments(raw json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing data")
	}
	return value, nil
}

func mapToolResult(req toolcontract.ToolCallRequest, result *mcpsdk.CallToolResult) toolcontract.ToolResult {
	if result == nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    "mcp tool returned empty result",
			Err:        errors.New("mcp tool returned empty result"),
		}
	}

	status := toolcontract.StatusSuccess
	var resultErr error
	if result.IsError {
		status = toolcontract.StatusExecutionError
		resultErr = errors.New("mcp tool returned error")
	}

	metadata := map[string]any{}
	if result.StructuredContent != nil {
		metadata["structured_content"] = result.StructuredContent
	}
	nonTextContent := nonTextContents(result.Content)
	if len(nonTextContent) > 0 {
		metadata["mcp_content"] = nonTextContent
	}
	if len(metadata) == 0 {
		metadata = nil
	}

	return toolcontract.ToolResult{
		ToolCallID: req.ToolCallID,
		Name:       req.Name,
		Status:     status,
		Content:    strings.Join(textContents(result.Content), "\n"),
		Metadata:   metadata,
		Err:        resultErr,
	}
}

func textContents(contents []mcpsdk.Content) []string {
	texts := make([]string, 0, len(contents))
	for _, content := range contents {
		if text, ok := content.(mcpsdk.TextContent); ok {
			texts = append(texts, text.Text)
		}
	}
	return texts
}

func nonTextContents(contents []mcpsdk.Content) []any {
	items := make([]any, 0, len(contents))
	for _, content := range contents {
		if _, ok := content.(mcpsdk.TextContent); ok {
			continue
		}
		items = append(items, content)
	}
	return items
}
