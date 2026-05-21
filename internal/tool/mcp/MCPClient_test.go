package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestNewClientValidatesConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "stdio requires command",
			cfg: Config{
				Kind: TransportStdio,
			},
			wantErr: "stdio command is required",
		},
		{
			name: "streamable http requires url",
			cfg: Config{
				Kind: TransportStreamableHTTP,
			},
			wantErr: "streamable_http url is required",
		},
		{
			name: "unknown transport kind",
			cfg: Config{
				Kind: TransportKind("websocket"),
			},
			wantErr: "unsupported mcp transport",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.cfg)
			require.Nil(t, c)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestListToolsMapsMCPMetadata(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`)
	client := &MCPClient{
		kind: TransportStreamableHTTP,
		session: &fakeSession{
			listToolsResult: &mcpsdk.ListToolsResult{
				Tools: []mcpsdk.Tool{
					{
						Name:           "search",
						Description:    "Search remote index",
						RawInputSchema: schema,
					},
				},
			},
		},
	}

	tools, err := client.ListTools(context.Background())

	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "search", tools[0].Name)
	require.Equal(t, "Search remote index", tools[0].Description)
	require.JSONEq(t, string(schema), string(tools[0].Parameters))
	require.True(t, tools[0].Enabled)
	require.Equal(t, toolcontract.AttentionLevel, tools[0].SecurityLevel)
	require.False(t, tools[0].IsConcurrencySafe)
	require.Equal(t, toolcontract.RequireNetwork, tools[0].Requirements)
}

func TestCallToolMapsResult(t *testing.T) {
	t.Run("success text result", func(t *testing.T) {
		session := &fakeSession{
			callToolResult: mcpsdk.NewToolResultText("done"),
		}
		client := &MCPClient{session: session}

		result := client.CallTool(context.Background(), toolcontract.ToolCallRequest{
			ToolCallID: "call-1",
			Name:       "search",
			Arguments:  json.RawMessage(`{"query":"agentGo"}`),
		})

		require.Equal(t, "call-1", result.ToolCallID)
		require.Equal(t, "search", result.Name)
		require.Equal(t, toolcontract.StatusSuccess, result.Status)
		require.Equal(t, "done", result.Content)
		require.NoError(t, result.Err)
		require.Equal(t, "search", session.callToolRequest.Params.Name)
		require.Equal(t, map[string]any{"query": "agentGo"}, session.callToolRequest.Params.Arguments)
	})

	t.Run("tool error result", func(t *testing.T) {
		client := &MCPClient{
			session: &fakeSession{
				callToolResult: mcpsdk.NewToolResultError("failed remotely"),
			},
		}

		result := client.CallTool(context.Background(), toolcontract.ToolCallRequest{
			ToolCallID: "call-2",
			Name:       "search",
			Arguments:  json.RawMessage(`{}`),
		})

		require.Equal(t, toolcontract.StatusExecutionError, result.Status)
		require.Equal(t, "failed remotely", result.Content)
		require.ErrorContains(t, result.Err, "mcp tool returned error")
	})

	t.Run("protocol error", func(t *testing.T) {
		client := &MCPClient{
			session: &fakeSession{
				callToolErr: errors.New("server unavailable"),
			},
		}

		result := client.CallTool(context.Background(), toolcontract.ToolCallRequest{
			ToolCallID: "call-3",
			Name:       "search",
			Arguments:  json.RawMessage(`{}`),
		})

		require.Equal(t, toolcontract.StatusSystemError, result.Status)
		require.Equal(t, "mcp tool call failed", result.Content)
		require.ErrorContains(t, result.Err, "server unavailable")
	})

	t.Run("structured content metadata", func(t *testing.T) {
		client := &MCPClient{
			session: &fakeSession{
				callToolResult: mcpsdk.NewToolResultStructured(map[string]any{"total": float64(2)}, "found 2"),
			},
		}

		result := client.CallTool(context.Background(), toolcontract.ToolCallRequest{
			ToolCallID: "call-4",
			Name:       "search",
			Arguments:  json.RawMessage(`{}`),
		})

		require.Equal(t, toolcontract.StatusSuccess, result.Status)
		require.Equal(t, "found 2", result.Content)
		require.Equal(t, map[string]any{"total": float64(2)}, result.Metadata["structured_content"])
	})
}

func TestClientLifecycle(t *testing.T) {
	t.Run("start initializes after transport start", func(t *testing.T) {
		session := &fakeSession{}
		client := &MCPClient{
			name:    "external",
			session: session,
		}

		err := client.Start(context.Background())

		require.NoError(t, err)
		require.Equal(t, []string{"start", "initialize"}, session.lifecycleCalls)
		require.Equal(t, mcpsdk.LATEST_PROTOCOL_VERSION, session.initializeRequest.Params.ProtocolVersion)
		require.Equal(t, "agentGo", session.initializeRequest.Params.ClientInfo.Name)
		require.Equal(t, "external", session.initializeRequest.Params.ClientInfo.Title)
	})

	t.Run("close forwards to session", func(t *testing.T) {
		session := &fakeSession{}
		client := &MCPClient{session: session}

		err := client.Close()

		require.NoError(t, err)
		require.Equal(t, []string{"close"}, session.lifecycleCalls)
	})

	t.Run("start returns canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		client := &MCPClient{session: &fakeSession{}}

		err := client.Start(ctx)

		require.ErrorIs(t, err, context.Canceled)
	})
}

type fakeSession struct {
	listToolsResult   *mcpsdk.ListToolsResult
	lifecycleCalls    []string
	initializeRequest mcpsdk.InitializeRequest
	callToolRequest   mcpsdk.CallToolRequest
	callToolResult    *mcpsdk.CallToolResult
	callToolErr       error
}

func (s *fakeSession) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.lifecycleCalls = append(s.lifecycleCalls, "start")
	return nil
}

func (s *fakeSession) Initialize(_ context.Context, request mcpsdk.InitializeRequest) (*mcpsdk.InitializeResult, error) {
	s.lifecycleCalls = append(s.lifecycleCalls, "initialize")
	s.initializeRequest = request
	return &mcpsdk.InitializeResult{}, nil
}

func (s *fakeSession) ListTools(context.Context, mcpsdk.ListToolsRequest) (*mcpsdk.ListToolsResult, error) {
	return s.listToolsResult, nil
}

func (s *fakeSession) CallTool(_ context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	s.callToolRequest = request
	return s.callToolResult, s.callToolErr
}

func (s *fakeSession) Close() error {
	s.lifecycleCalls = append(s.lifecycleCalls, "close")
	return nil
}

func TestNewClientBuildsSupportedTransports(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "stdio",
			cfg: Config{
				Name:    "local",
				Kind:    TransportStdio,
				Command: "mcp-server",
				Args:    []string{"--debug"},
				Env:     []string{"MCP_ENV=test"},
			},
		},
		{
			name: "streamable http",
			cfg: Config{
				Name: "remote",
				Kind: TransportStreamableHTTP,
				URL:  "http://127.0.0.1:8080/mcp",
				Headers: map[string]string{
					"Authorization": "Bearer test",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.cfg)
			require.NoError(t, err)
			require.NotNil(t, c)
			require.NotNil(t, c.client)
			require.Equal(t, tt.cfg.Kind, c.kind)
			require.Equal(t, tt.cfg.Name, c.name)
		})
	}
}
