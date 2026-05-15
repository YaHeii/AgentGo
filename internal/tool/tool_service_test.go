package tool

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"github.com/stretchr/testify/require"
)

func TestCallReturnsPerRequestResultsInInputOrder(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: toolcontract.Metadata{Name: "grep", Enabled: true, IsConcurrencySafe: true},
			execute: func(_ context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
				return toolcontract.ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     toolcontract.StatusSuccess,
					Content:    string(req.Arguments),
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), toolcontract.BatchRequest{
		Calls: []toolcontract.ToolCallRequest{
			{ToolCallID: "call_1", Name: "grep", Arguments: json.RawMessage(`{"q":"first"}`), PermissionLevel: toolcontract.SafeLevel},
			{ToolCallID: "call_2", Name: "grep", Arguments: json.RawMessage(`{"q":"second"}`), PermissionLevel: toolcontract.SafeLevel},
		},
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, "call_1", results[0].ToolCallID)
	require.Equal(t, "call_2", results[1].ToolCallID)
	require.Equal(t, `{"q":"first"}`, results[0].Content)
	require.Equal(t, `{"q":"second"}`, results[1].Content)
}

func TestCallReturnsErrorForUnknownTool(t *testing.T) {
	svc := NewService()

	results, err := svc.Call(context.Background(), toolcontract.BatchRequest{
		Calls: []toolcontract.ToolCallRequest{
			{ToolCallID: "call_1", Name: "missing", Arguments: json.RawMessage(`{}`), PermissionLevel: toolcontract.SafeLevel},
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
}

func TestCallReturnsErrorWhenRequiredWorkingDirMissing(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: toolcontract.Metadata{
				Name:              "grep",
				Enabled:           true,
				IsConcurrencySafe: true,
				Requirements:      toolcontract.RequireWorkingDir,
			},
			execute: func(_ context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
				return toolcontract.ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     toolcontract.StatusSuccess,
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), toolcontract.BatchRequest{
		Calls: []toolcontract.ToolCallRequest{
			{ToolCallID: "call_1", Name: "grep", Arguments: json.RawMessage(`{}`), PermissionLevel: toolcontract.SafeLevel},
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
}

func TestCallReturnsErrorForDisabledTool(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: toolcontract.Metadata{Name: "grep", Enabled: false, IsConcurrencySafe: true},
			execute: func(_ context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
				return toolcontract.ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     toolcontract.StatusSuccess,
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), toolcontract.BatchRequest{
		Calls: []toolcontract.ToolCallRequest{{
			ToolCallID:      "call_1",
			Name:            "grep",
			Arguments:       json.RawMessage(`{}`),
			PermissionLevel: toolcontract.SafeLevel,
			Context: toolcontract.ToolCallContext{
				WorkingDir: "/workspace",
			},
		}},
	})
	require.Nil(t, results)
	require.Error(t, err)
}

func TestCallReturnsErrorWhenRequiredWorkspaceRootMissing(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: toolcontract.Metadata{
				Name:              "grep",
				Enabled:           true,
				IsConcurrencySafe: true,
				Requirements:      toolcontract.RequireWorkspaceRoot,
			},
			execute: func(_ context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
				return toolcontract.ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     toolcontract.StatusSuccess,
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), toolcontract.BatchRequest{
		Calls: []toolcontract.ToolCallRequest{
			{ToolCallID: "call_1", Name: "grep", Arguments: json.RawMessage(`{}`), PermissionLevel: toolcontract.SafeLevel},
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
}

func TestCallReturnsErrorWhenArgumentsDoNotMatchSchema(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: toolcontract.Metadata{
				Name:              "grep",
				Enabled:           true,
				IsConcurrencySafe: true,
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"pattern": { "type": "string" },
						"literal_text": { "type": "boolean" }
					},
					"required": ["pattern"]
				}`),
			},
			execute: func(_ context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
				return toolcontract.ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     toolcontract.StatusSuccess,
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), toolcontract.BatchRequest{
		Calls: []toolcontract.ToolCallRequest{
			{ToolCallID: "call_1", Name: "grep", Arguments: json.RawMessage(`{"literal_text":"yes"}`), PermissionLevel: toolcontract.SafeLevel},
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
}

func TestCallDoesNotExecuteAnyToolWhenLaterCallFailsSchemaValidation(t *testing.T) {
	var executed bool

	svc := NewService(
		&stubTool{
			meta: toolcontract.Metadata{
				Name:              "grep",
				Enabled:           true,
				IsConcurrencySafe: true,
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"pattern": { "type": "string" },
						"literal_text": { "type": "boolean" }
					},
					"required": ["pattern"]
				}`),
			},
			execute: func(_ context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
				executed = true
				return toolcontract.ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     toolcontract.StatusSuccess,
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), toolcontract.BatchRequest{
		Calls: []toolcontract.ToolCallRequest{
			{ToolCallID: "call_1", Name: "grep", Arguments: json.RawMessage(`{"pattern":"ok"}`), PermissionLevel: toolcontract.SafeLevel},
			{ToolCallID: "call_2", Name: "grep", Arguments: json.RawMessage(`{"literal_text":"yes"}`), PermissionLevel: toolcontract.SafeLevel},
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
	require.False(t, executed)
}

func TestCallSerializesNonConcurrentToolAcrossGoroutines(t *testing.T) {
	var (
		mu            sync.Mutex
		active        int
		maxActive     int
		executionSeen []string
	)

	svc := NewService(
		&stubTool{
			meta: toolcontract.Metadata{Name: "serial", Enabled: true, IsConcurrencySafe: false},
			execute: func(_ context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
				mu.Lock()
				active++
				if active > maxActive {
					maxActive = active
				}
				executionSeen = append(executionSeen, req.ToolCallID)
				mu.Unlock()

				time.Sleep(40 * time.Millisecond)

				mu.Lock()
				active--
				mu.Unlock()

				return toolcontract.ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     toolcontract.StatusSuccess,
				}
			},
		},
	)

	req1 := toolcontract.BatchRequest{Calls: []toolcontract.ToolCallRequest{
		{ToolCallID: "call_1", Name: "serial", Arguments: json.RawMessage(`{}`), PermissionLevel: toolcontract.SafeLevel},
	}}
	req2 := toolcontract.BatchRequest{Calls: []toolcontract.ToolCallRequest{
		{ToolCallID: "call_2", Name: "serial", Arguments: json.RawMessage(`{}`), PermissionLevel: toolcontract.SafeLevel},
	}}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = svc.Call(context.Background(), req1)
	}()
	go func() {
		defer wg.Done()
		_, _ = svc.Call(context.Background(), req2)
	}()
	wg.Wait()

	require.Equal(t, 1, maxActive)
	require.Len(t, executionSeen, 2)
}

func TestCallAllowsConcurrentSafeToolToRunInParallel(t *testing.T) {
	var (
		mu        sync.Mutex
		active    int
		maxActive int
	)

	svc := NewService(
		&stubTool{
			meta: toolcontract.Metadata{Name: "parallel", Enabled: true, IsConcurrencySafe: true},
			execute: func(_ context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
				mu.Lock()
				active++
				if active > maxActive {
					maxActive = active
				}
				mu.Unlock()

				time.Sleep(40 * time.Millisecond)

				mu.Lock()
				active--
				mu.Unlock()

				return toolcontract.ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     toolcontract.StatusSuccess,
				}
			},
		},
	)

	_, err := svc.Call(context.Background(), toolcontract.BatchRequest{
		Calls: []toolcontract.ToolCallRequest{
			{ToolCallID: "call_1", Name: "parallel", Arguments: json.RawMessage(`{}`), PermissionLevel: toolcontract.SafeLevel},
			{ToolCallID: "call_2", Name: "parallel", Arguments: json.RawMessage(`{}`), PermissionLevel: toolcontract.SafeLevel},
		},
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, maxActive, 2)
}

func TestListToolsFiltersByPermissionLevel(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: toolcontract.Metadata{Name: "safe", Enabled: true, SecurityLevel: toolcontract.SafeLevel},
		},
		&stubTool{
			meta: toolcontract.Metadata{Name: "attention", Enabled: true, SecurityLevel: toolcontract.AttentionLevel},
		},
		&stubTool{
			meta: toolcontract.Metadata{Name: "danger", Enabled: true, SecurityLevel: toolcontract.DangerLevel},
		},
	)

	metas := svc.ListTools(context.Background(), toolcontract.AttentionLevel)
	require.Len(t, metas, 2)
	require.Equal(t, "attention", metas[0].Name)
	require.Equal(t, "safe", metas[1].Name)
}

func TestToolCallRequestCarriesExecutionContext(t *testing.T) {
	req := toolcontract.ToolCallRequest{
		ToolCallID:      "call_1",
		Name:            "grep",
		Arguments:       json.RawMessage(`{"q":"hello"}`),
		PermissionLevel: toolcontract.AttentionLevel,
		Context: toolcontract.ToolCallContext{
			SessionID:     "session-1",
			TurnID:        "turn-1",
			WorkspaceRoot: "/workspace",
			WorkingDir:    "/workspace/internal",
		},
	}

	require.Equal(t, "call_1", req.ToolCallID)
	require.Equal(t, "grep", req.Name)
	require.Equal(t, toolcontract.AttentionLevel, req.PermissionLevel)
	require.Equal(t, "session-1", req.Context.SessionID)
	require.Equal(t, "/workspace/internal", req.Context.WorkingDir)
}

func TestCallReturnsErrorWhenPermissionLevelIsLowerThanToolRequirement(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: toolcontract.Metadata{
				Name:              "dangerous",
				Enabled:           true,
				SecurityLevel:     toolcontract.DangerLevel,
				IsConcurrencySafe: true,
			},
			execute: func(_ context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
				return toolcontract.ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     toolcontract.StatusSuccess,
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), toolcontract.BatchRequest{
		Calls: []toolcontract.ToolCallRequest{
			{ToolCallID: "call_1", Name: "dangerous", Arguments: json.RawMessage(`{}`), PermissionLevel: toolcontract.AttentionLevel},
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission_level")
}

func TestCallReturnsBatchErrorWhenContextAlreadyCanceled(t *testing.T) {
	svc := NewService(&stubTool{
		meta: toolcontract.Metadata{Name: "grep", Enabled: true, IsConcurrencySafe: true},
		execute: func(_ context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
			return toolcontract.ToolResult{
				ToolCallID: req.ToolCallID,
				Name:       req.Name,
				Status:     toolcontract.StatusSuccess,
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, err := svc.Call(ctx, toolcontract.BatchRequest{
		Calls: []toolcontract.ToolCallRequest{
			{ToolCallID: "call_1", Name: "grep", Arguments: json.RawMessage(`{}`), PermissionLevel: toolcontract.SafeLevel},
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
}

type stubTool struct {
	meta    toolcontract.Metadata
	execute func(ctx context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult
}

func (s *stubTool) Metadata() toolcontract.Metadata {
	return s.meta
}

func (s *stubTool) Execute(ctx context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
	if s.execute == nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusExecutionError,
			Err:        errors.New("stub execute not configured"),
		}
	}
	return s.execute(ctx, req)
}
