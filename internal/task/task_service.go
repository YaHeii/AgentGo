package task

import (
	"context"
	"time"

	taskcontract "github.com/YaHeii/agentGo/internal/task/contract"
)

var ErrTaskNotFound = taskcontract.ErrTaskNotFound

type TaskService struct {
	taskStore taskStore
	nowFunc   func() time.Time
}

func NewTaskService(st taskStore) *TaskService {
	return &TaskService{
		taskStore: st,
		nowFunc:   time.Now,
	}
}

func (s *TaskService) Create(ctx context.Context, params taskcontract.CreateTaskParams) (taskcontract.Task, error) {
	params.CreatedAt = normalizeOrNow(params.CreatedAt, s.nowFunc)
	return s.taskStore.CreateTask(ctx, params)
}

func (s *TaskService) Get(ctx context.Context, subagentSessionID string) (taskcontract.Task, error) {
	return s.taskStore.GetTask(ctx, subagentSessionID)
}

func (s *TaskService) ListByParentSession(ctx context.Context, parentSessionID string) ([]taskcontract.Task, error) {
	return s.taskStore.ListTasksByParentSession(ctx, parentSessionID)
}

func (s *TaskService) UpdateProgress(ctx context.Context, params taskcontract.UpdateTaskProgressParams) (taskcontract.Task, error) {
	return s.taskStore.UpdateTaskProgress(ctx, params)
}

func (s *TaskService) Complete(ctx context.Context, params taskcontract.CompleteTaskParams) (taskcontract.Task, error) {
	params.CompletedAt = normalizeOrNow(params.CompletedAt, s.nowFunc)
	return s.taskStore.CompleteTask(ctx, params)
}

func (s *TaskService) Fail(ctx context.Context, params taskcontract.FailTaskParams) (taskcontract.Task, error) {
	params.CompletedAt = normalizeOrNow(params.CompletedAt, s.nowFunc)
	return s.taskStore.FailTask(ctx, params)
}

func normalizeOrNow(ts time.Time, nowFunc func() time.Time) time.Time {
	if ts.IsZero() {
		return nowFunc().UTC()
	}
	return ts.UTC()
}
