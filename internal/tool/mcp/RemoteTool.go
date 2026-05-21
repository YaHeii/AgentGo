package mcp

import (
	"context"
	"strings"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

const toolNameSeparator = "__"

type RemoteTool struct {
	serverName string
	client     Client
	meta       toolcontract.Metadata
}

func NewRemoteTool(serverName string, client Client, meta toolcontract.Metadata) *RemoteTool {
	meta.Name = prefixedToolName(serverName, meta.Name)
	return &RemoteTool{
		serverName: strings.TrimSpace(serverName),
		client:     client,
		meta:       meta,
	}
}

func (t *RemoteTool) Metadata() toolcontract.Metadata {
	return t.meta
}

func (t *RemoteTool) Execute(ctx context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
	original := req
	original.Name = t.originalName()

	result := t.client.CallTool(ctx, original)
	result.Name = t.meta.Name
	return result
}

func (t *RemoteTool) originalName() string {
	prefix := t.serverName + toolNameSeparator
	return strings.TrimPrefix(t.meta.Name, prefix)
}

func prefixedToolName(serverName string, name string) string {
	serverName = strings.TrimSpace(serverName)
	name = strings.TrimSpace(name)
	if serverName == "" {
		return name
	}
	if strings.HasPrefix(name, serverName+toolNameSeparator) {
		return name
	}
	return serverName + toolNameSeparator + name
}
