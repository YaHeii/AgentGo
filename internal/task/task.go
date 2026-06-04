package task

import (
	"context"

	taskcontract "github.com/YaHeii/agentGo/internal/task/contract"
)

type taskStore interface {
	CreateTask(ctx context.Context, params taskcontract.CreateTaskParams) (taskcontract.Task, error)
	GetTask(ctx context.Context, subagentSessionID string) (taskcontract.Task, error)
	ListTasksByParentSession(ctx context.Context, parentSessionID string) ([]taskcontract.Task, error)
	UpdateTaskProgress(ctx context.Context, params taskcontract.UpdateTaskProgressParams) (taskcontract.Task, error)
	CompleteTask(ctx context.Context, params taskcontract.CompleteTaskParams) (taskcontract.Task, error)
	FailTask(ctx context.Context, params taskcontract.FailTaskParams) (taskcontract.Task, error)
}
