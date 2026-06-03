package tool

// TODO:
// - Reintroduce tighter integration with a richer session todo domain model if agentGo adds one.
// - Reintroduce event emission dedicated to todo transitions if the UI later needs separate todo events.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

const TodosToolName = "todos"

type TodosParams struct {
	Todos []TodoItem `json:"todos"`
}

type TodoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"active_form"`
}

type todosSessionStore interface {
	Get(ctx context.Context, id string) (sessioncontract.Session, error)
	Save(ctx context.Context, session sessioncontract.Session) (sessioncontract.Session, error)
}

type TodosTool struct {
	sessions todosSessionStore
}

func NewTodosTool(sessions todosSessionStore) *TodosTool {
	return &TodosTool{sessions: sessions}
}

func (t *TodosTool) Metadata() toolcontract.Metadata {
	return toolcontract.Metadata{
		Name:        TodosToolName,
		Description: "用一组完整的 todo 项替换当前会话的 todo 列表，用于同步执行计划、当前进行项和完成状态。调用时应提交期望的完整列表，而不是只提交增量变更。",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"todos": {
					"type": "array",
					"description": "要写回当前会话的完整 todo 列表。每个元素表示一个任务及其当前状态；本次提交会覆盖旧列表。",
					"items": {
						"type": "object",
						"properties": {
							"content": { "type": "string", "description": "todo 的稳定描述文本，用于标识这项任务本身。" },
							"status": { "type": "string", "description": "todo 当前状态，只允许 'pending'、'in_progress' 或 'completed'。" },
							"active_form": { "type": "string", "description": "可选的进行时表达，用于展示当前正在执行的动作，例如 'Writing tests'。当 status 为 'in_progress' 时建议提供。" }
						},
						"required": ["content", "status"]
					}
				}
			},
			"required": ["todos"]
		}`),
		Enabled:           true,
		SecurityLevel:     toolcontract.SafeLevel,
		IsConcurrencySafe: false,
	}
}

func (t *TodosTool) Execute(ctx context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
	var params TodosParams
	if err := json.Unmarshal(req.Arguments, &params); err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    "failed to decode todo arguments",
			Err:        err,
		}
	}

	sessionID := strings.TrimSpace(req.Context.SessionID)
	if sessionID == "" {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusValidationFailed,
			Content:    "session_id is required for todos tool",
			Err:        fmt.Errorf("missing session_id"),
		}
	}
	if t.sessions == nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    "todos session store is not configured",
			Err:        fmt.Errorf("missing todos session store"),
		}
	}

	current, err := t.sessions.Get(ctx, sessionID)
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusExecutionError,
			Content:    "failed to load session",
			Err:        err,
		}
	}

	var previous []TodoItem
	if strings.TrimSpace(current.TodosJSON) != "" {
		_ = json.Unmarshal([]byte(current.TodosJSON), &previous)
	}
	oldStatusByContent := make(map[string]string, len(previous))
	for _, item := range previous {
		oldStatusByContent[item.Content] = item.Status
	}

	completedCount := 0
	var justCompleted []string
	justStarted := ""
	for _, item := range params.Todos {
		switch item.Status {
		case "pending", "in_progress", "completed":
		default:
			return toolcontract.ToolResult{
				ToolCallID: req.ToolCallID,
				Name:       req.Name,
				Status:     toolcontract.StatusValidationFailed,
				Content:    fmt.Sprintf("invalid status %q for todo %q", item.Status, item.Content),
				Err:        fmt.Errorf("invalid status %q", item.Status),
			}
		}

		oldStatus, existed := oldStatusByContent[item.Content]
		if item.Status == "completed" {
			completedCount++
			if existed && oldStatus != "completed" {
				justCompleted = append(justCompleted, item.Content)
			}
		}
		if item.Status == "in_progress" && (!existed || oldStatus != "in_progress") {
			if strings.TrimSpace(item.ActiveForm) != "" {
				justStarted = item.ActiveForm
			} else {
				justStarted = item.Content
			}
		}
	}

	rawTodos, err := json.Marshal(params.Todos)
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    "failed to encode todos",
			Err:        err,
		}
	}

	current.TodosJSON = string(rawTodos)
	saved, err := t.sessions.Save(ctx, current)
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusExecutionError,
			Content:    "failed to save todos",
			Err:        err,
		}
	}

	pendingCount := 0
	inProgressCount := 0
	for _, item := range params.Todos {
		switch item.Status {
		case "pending":
			pendingCount++
		case "in_progress":
			inProgressCount++
		}
	}

	return toolcontract.ToolResult{
		ToolCallID: req.ToolCallID,
		Name:       req.Name,
		Status:     toolcontract.StatusSuccess,
		Content: fmt.Sprintf(
			"Todo list updated successfully.\n\nStatus: %d pending, %d in progress, %d completed",
			pendingCount,
			inProgressCount,
			completedCount,
		),
		Metadata: map[string]any{
			"is_new":         len(previous) == 0,
			"todos":          params.Todos,
			"just_completed": justCompleted,
			"just_started":   justStarted,
			"completed":      completedCount,
			"total":          len(params.Todos),
			"session_id":     saved.ID,
		},
	}
}
