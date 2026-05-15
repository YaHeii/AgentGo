package tool

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCallReturnsPerRequestResultsInInputOrder(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: Metadata{Name: "grep", Enabled: true, IsConcurrencySafe: true},
			execute: func(_ context.Context, req ToolCallRequest) ToolResult {
				return ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     StatusSuccess,
					Content:    string(req.Arguments),
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), BatchRequest{
		Calls: []ToolCallRequest{
			NewToolCallRequest("call_1", "grep", json.RawMessage(`{"q":"first"}`), SafeLevel, ToolCallContext{}),
			NewToolCallRequest("call_2", "grep", json.RawMessage(`{"q":"second"}`), SafeLevel, ToolCallContext{}),
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

	results, err := svc.Call(context.Background(), BatchRequest{
		Calls: []ToolCallRequest{
			NewToolCallRequest("call_1", "missing", json.RawMessage(`{}`), SafeLevel, ToolCallContext{}),
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
}

func TestCallReturnsErrorWhenRequiredWorkingDirMissing(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: Metadata{
				Name:              "grep",
				Enabled:           true,
				IsConcurrencySafe: true,
				Requirements:      RequireWorkingDir,
			},
			execute: func(_ context.Context, req ToolCallRequest) ToolResult {
				return ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     StatusSuccess,
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), BatchRequest{
		Calls: []ToolCallRequest{
			NewToolCallRequest("call_1", "grep", json.RawMessage(`{}`), SafeLevel, ToolCallContext{}),
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
}

func TestCallReturnsErrorForDisabledTool(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: Metadata{Name: "grep", Enabled: false, IsConcurrencySafe: true},
			execute: func(_ context.Context, req ToolCallRequest) ToolResult {
				return ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     StatusSuccess,
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), BatchRequest{
		Calls: []ToolCallRequest{
			NewToolCallRequest("call_1", "grep", json.RawMessage(`{}`), SafeLevel, ToolCallContext{
				WorkingDir: "/workspace",
			}),
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
}

func TestCallReturnsErrorWhenRequiredWorkspaceRootMissing(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: Metadata{
				Name:              "grep",
				Enabled:           true,
				IsConcurrencySafe: true,
				Requirements:      RequireWorkspaceRoot,
			},
			execute: func(_ context.Context, req ToolCallRequest) ToolResult {
				return ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     StatusSuccess,
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), BatchRequest{
		Calls: []ToolCallRequest{
			NewToolCallRequest("call_1", "grep", json.RawMessage(`{}`), SafeLevel, ToolCallContext{}),
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
}

func TestCallReturnsErrorWhenArgumentsDoNotMatchSchema(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: Metadata{
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
			execute: func(_ context.Context, req ToolCallRequest) ToolResult {
				return ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     StatusSuccess,
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), BatchRequest{
		Calls: []ToolCallRequest{
			NewToolCallRequest("call_1", "grep", json.RawMessage(`{"literal_text":"yes"}`), SafeLevel, ToolCallContext{}),
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
}

func TestCallDoesNotExecuteAnyToolWhenLaterCallFailsSchemaValidation(t *testing.T) {
	var executed bool

	svc := NewService(
		&stubTool{
			meta: Metadata{
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
			execute: func(_ context.Context, req ToolCallRequest) ToolResult {
				executed = true
				return ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     StatusSuccess,
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), BatchRequest{
		Calls: []ToolCallRequest{
			NewToolCallRequest("call_1", "grep", json.RawMessage(`{"pattern":"ok"}`), SafeLevel, ToolCallContext{}),
			NewToolCallRequest("call_2", "grep", json.RawMessage(`{"literal_text":"yes"}`), SafeLevel, ToolCallContext{}),
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
			meta: Metadata{Name: "serial", Enabled: true, IsConcurrencySafe: false},
			execute: func(_ context.Context, req ToolCallRequest) ToolResult {
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

				return ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     StatusSuccess,
				}
			},
		},
	)

	req1 := BatchRequest{Calls: []ToolCallRequest{
		NewToolCallRequest("call_1", "serial", json.RawMessage(`{}`), SafeLevel, ToolCallContext{}),
	}}
	req2 := BatchRequest{Calls: []ToolCallRequest{
		NewToolCallRequest("call_2", "serial", json.RawMessage(`{}`), SafeLevel, ToolCallContext{}),
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
			meta: Metadata{Name: "parallel", Enabled: true, IsConcurrencySafe: true},
			execute: func(_ context.Context, req ToolCallRequest) ToolResult {
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

				return ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     StatusSuccess,
				}
			},
		},
	)

	_, err := svc.Call(context.Background(), BatchRequest{
		Calls: []ToolCallRequest{
			NewToolCallRequest("call_1", "parallel", json.RawMessage(`{}`), SafeLevel, ToolCallContext{}),
			NewToolCallRequest("call_2", "parallel", json.RawMessage(`{}`), SafeLevel, ToolCallContext{}),
		},
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, maxActive, 2)
}

func TestListToolsFiltersByPermissionLevel(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: Metadata{Name: "safe", Enabled: true, SecurityLevel: SafeLevel},
		},
		&stubTool{
			meta: Metadata{Name: "attention", Enabled: true, SecurityLevel: AttentionLevel},
		},
		&stubTool{
			meta: Metadata{Name: "danger", Enabled: true, SecurityLevel: DangerLevel},
		},
	)

	metas := svc.ListTools(context.Background(), AttentionLevel)
	require.Len(t, metas, 2)
	require.Equal(t, "attention", metas[0].Name)
	require.Equal(t, "safe", metas[1].Name)
}

func TestNewToolCallRequestCarriesExecutionContext(t *testing.T) {
	req := NewToolCallRequest(
		"call_1",
		"grep",
		json.RawMessage(`{"q":"hello"}`),
		AttentionLevel,
		ToolCallContext{
			SessionID:     "session-1",
			TurnID:        "turn-1",
			WorkspaceRoot: "/workspace",
			WorkingDir:    "/workspace/internal",
		},
	)

	require.Equal(t, "call_1", req.ToolCallID)
	require.Equal(t, "grep", req.Name)
	require.Equal(t, AttentionLevel, req.PermissionLevel)
	require.Equal(t, "session-1", req.Context.SessionID)
	require.Equal(t, "/workspace/internal", req.Context.WorkingDir)
}

func TestCallReturnsErrorWhenPermissionLevelIsLowerThanToolRequirement(t *testing.T) {
	svc := NewService(
		&stubTool{
			meta: Metadata{
				Name:              "dangerous",
				Enabled:           true,
				SecurityLevel:     DangerLevel,
				IsConcurrencySafe: true,
			},
			execute: func(_ context.Context, req ToolCallRequest) ToolResult {
				return ToolResult{
					ToolCallID: req.ToolCallID,
					Name:       req.Name,
					Status:     StatusSuccess,
				}
			},
		},
	)

	results, err := svc.Call(context.Background(), BatchRequest{
		Calls: []ToolCallRequest{
			NewToolCallRequest("call_1", "dangerous", json.RawMessage(`{}`), AttentionLevel, ToolCallContext{}),
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission_level")
}

func TestCallReturnsBatchErrorWhenContextAlreadyCanceled(t *testing.T) {
	svc := NewService(&stubTool{
		meta: Metadata{Name: "grep", Enabled: true, IsConcurrencySafe: true},
		execute: func(_ context.Context, req ToolCallRequest) ToolResult {
			return ToolResult{
				ToolCallID: req.ToolCallID,
				Name:       req.Name,
				Status:     StatusSuccess,
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, err := svc.Call(ctx, BatchRequest{
		Calls: []ToolCallRequest{
			NewToolCallRequest("call_1", "grep", json.RawMessage(`{}`), SafeLevel, ToolCallContext{}),
		},
	})
	require.Nil(t, results)
	require.Error(t, err)
}

type stubTool struct {
	meta    Metadata
	execute func(ctx context.Context, req ToolCallRequest) ToolResult
}

func (s *stubTool) Metadata() Metadata {
	return s.meta
}

func (s *stubTool) Execute(ctx context.Context, req ToolCallRequest) ToolResult {
	if s.execute == nil {
		return ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     StatusExecutionError,
			Err:        errors.New("stub execute not configured"),
		}
	}
	return s.execute(ctx, req)
}
