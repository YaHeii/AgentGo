package contract

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToolCallRequestCarriesExecutionContext(t *testing.T) {
	req := ToolCallRequest{
		ToolCallID:      "call_1",
		Name:            "grep",
		Arguments:       json.RawMessage(`{"pattern":"hello"}`),
		PermissionLevel: AttentionLevel,
		Context: ToolCallContext{
			SessionID:     "session-1",
			TurnID:        "turn-1",
			WorkspaceRoot: "/workspace",
			WorkingDir:    "/workspace/internal",
		},
	}

	require.Equal(t, "call_1", req.ToolCallID)
	require.Equal(t, "grep", req.Name)
	require.Equal(t, AttentionLevel, req.PermissionLevel)
	require.Equal(t, "session-1", req.Context.SessionID)
	require.Equal(t, "/workspace/internal", req.Context.WorkingDir)
}

func TestBatchRequestPreservesInputOrder(t *testing.T) {
	batch := BatchRequest{
		Calls: []ToolCallRequest{
			{ToolCallID: "call_1", Name: "grep"},
			{ToolCallID: "call_2", Name: "grep"},
		},
	}

	require.Len(t, batch.Calls, 2)
	require.Equal(t, "call_1", batch.Calls[0].ToolCallID)
	require.Equal(t, "call_2", batch.Calls[1].ToolCallID)
}
