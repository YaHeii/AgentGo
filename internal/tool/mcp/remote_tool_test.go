package mcp

import (
	"context"
	"encoding/json"
	"testing"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestRemoteToolMetadataUsesServerPrefixedName(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
	client := &MCPClient{}

	tool := NewRemoteTool("fs", client, toolcontract.Metadata{
		Name:              "read_file",
		Description:       "Read a file",
		Parameters:        schema,
		Enabled:           true,
		SecurityLevel:     toolcontract.AttentionLevel,
		IsConcurrencySafe: false,
		Requirements:      toolcontract.RequireNone,
	})

	meta := tool.Metadata()

	require.Equal(t, "fs__read_file", meta.Name)
	require.Equal(t, "Read a file", meta.Description)
	require.JSONEq(t, string(schema), string(meta.Parameters))
	require.True(t, meta.Enabled)
	require.Equal(t, toolcontract.AttentionLevel, meta.SecurityLevel)
}

func TestRemoteToolExecuteCallsOriginalMCPToolName(t *testing.T) {
	session := &fakeSession{
		callToolResult: mcpsdk.NewToolResultText("file content"),
	}
	client := &MCPClient{session: session}
	tool := NewRemoteTool("fs", client, toolcontract.Metadata{
		Name:        "read_file",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Enabled:     true,
		Description: "Read a file",
	})

	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID: "call-1",
		Name:       "fs__read_file",
		Arguments:  json.RawMessage(`{"path":"README.md"}`),
	})

	require.Equal(t, toolcontract.StatusSuccess, result.Status)
	require.Equal(t, "fs__read_file", result.Name)
	require.Equal(t, "file content", result.Content)
	require.Equal(t, "read_file", session.callToolRequest.Params.Name)
}
