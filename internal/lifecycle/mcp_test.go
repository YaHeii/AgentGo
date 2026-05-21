package lifecycle

import (
	"context"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

type stubMCPClient struct {
	tools  []toolcontract.Metadata
	closed bool
}

func (s *stubMCPClient) Start(context.Context) error {
	return nil
}

func (s *stubMCPClient) ListTools(context.Context) ([]toolcontract.Metadata, error) {
	return s.tools, nil
}

func (s *stubMCPClient) CallTool(_ context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
	return toolcontract.ToolResult{
		ToolCallID: req.ToolCallID,
		Name:       req.Name,
		Status:     toolcontract.StatusSuccess,
	}
}

func (s *stubMCPClient) Close() error {
	s.closed = true
	return nil
}
