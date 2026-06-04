package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	taskcontract "github.com/YaHeii/agentGo/internal/task/contract"
)

func (s *Store) CreateTask(ctx context.Context, params taskcontract.CreateTaskParams) (taskcontract.Task, error) {
	return createTaskWithQuerier(ctx, s.q, params)
}

func (s *Store) GetTask(ctx context.Context, subagentSessionID string) (taskcontract.Task, error) {
	return getTaskWithQuerier(ctx, s.q, subagentSessionID)
}

func (s *Store) ListTasksByParentSession(ctx context.Context, parentSessionID string) ([]taskcontract.Task, error) {
	return listTasksByParentSessionWithQuerier(ctx, s.q, parentSessionID)
}

func (s *Store) UpdateTaskProgress(ctx context.Context, params taskcontract.UpdateTaskProgressParams) (taskcontract.Task, error) {
	return updateTaskProgressWithQuerier(ctx, s.q, params)
}

func (s *Store) CompleteTask(ctx context.Context, params taskcontract.CompleteTaskParams) (taskcontract.Task, error) {
	return completeTaskWithQuerier(ctx, s.q, params)
}

func (s *Store) FailTask(ctx context.Context, params taskcontract.FailTaskParams) (taskcontract.Task, error) {
	return failTaskWithQuerier(ctx, s.q, params)
}

func (s *txStore) CreateTask(ctx context.Context, params taskcontract.CreateTaskParams) (taskcontract.Task, error) {
	return createTaskWithQuerier(ctx, s.q, params)
}

func (s *txStore) GetTask(ctx context.Context, subagentSessionID string) (taskcontract.Task, error) {
	return getTaskWithQuerier(ctx, s.q, subagentSessionID)
}

func (s *txStore) ListTasksByParentSession(ctx context.Context, parentSessionID string) ([]taskcontract.Task, error) {
	return listTasksByParentSessionWithQuerier(ctx, s.q, parentSessionID)
}

func (s *txStore) UpdateTaskProgress(ctx context.Context, params taskcontract.UpdateTaskProgressParams) (taskcontract.Task, error) {
	return updateTaskProgressWithQuerier(ctx, s.q, params)
}

func (s *txStore) CompleteTask(ctx context.Context, params taskcontract.CompleteTaskParams) (taskcontract.Task, error) {
	return completeTaskWithQuerier(ctx, s.q, params)
}

func (s *txStore) FailTask(ctx context.Context, params taskcontract.FailTaskParams) (taskcontract.Task, error) {
	return failTaskWithQuerier(ctx, s.q, params)
}

type taskQuerier interface {
	CreateTask(ctx context.Context, arg CreateTaskParams) (Task, error)
	GetTask(ctx context.Context, subagentSessionID string) (Task, error)
	ListTasksByParentSession(ctx context.Context, parentSessionID string) ([]Task, error)
	UpdateTaskProgress(ctx context.Context, arg UpdateTaskProgressParams) (Task, error)
	CompleteTask(ctx context.Context, arg CompleteTaskParams) (Task, error)
	FailTask(ctx context.Context, arg FailTaskParams) (Task, error)
}

func createTaskWithQuerier(ctx context.Context, q taskQuerier, params taskcontract.CreateTaskParams) (taskcontract.Task, error) {
	row, err := q.CreateTask(ctx, CreateTaskParams{
		SubagentSessionID:   params.SubagentSessionID,
		ParentSessionID:     params.ParentSessionID,
		Kind:                params.Kind,
		InputPayloadJson:    nullableString(params.InputPayloadJSON),
		ProgressPayloadJson: nullableString(params.ProgressPayloadJSON),
		ResultPayloadJson:   nullableString(params.ResultPayloadJSON),
		ErrorMessage:        params.ErrorMessage,
		CreatedAt:           params.CreatedAt.UTC().UnixMilli(),
	})
	if err != nil {
		return taskcontract.Task{}, err
	}
	return toTaskContract(row), nil
}

func getTaskWithQuerier(ctx context.Context, q taskQuerier, subagentSessionID string) (taskcontract.Task, error) {
	row, err := q.GetTask(ctx, subagentSessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return taskcontract.Task{}, taskcontract.ErrTaskNotFound
	}
	if err != nil {
		return taskcontract.Task{}, err
	}
	return toTaskContract(row), nil
}

func listTasksByParentSessionWithQuerier(ctx context.Context, q taskQuerier, parentSessionID string) ([]taskcontract.Task, error) {
	rows, err := q.ListTasksByParentSession(ctx, parentSessionID)
	if err != nil {
		return nil, err
	}

	tasks := make([]taskcontract.Task, 0, len(rows))
	for _, row := range rows {
		tasks = append(tasks, toTaskContract(row))
	}
	return tasks, nil
}

func updateTaskProgressWithQuerier(ctx context.Context, q taskQuerier, params taskcontract.UpdateTaskProgressParams) (taskcontract.Task, error) {
	row, err := q.UpdateTaskProgress(ctx, UpdateTaskProgressParams{
		ProgressPayloadJson: nullableString(params.ProgressPayloadJSON),
		SubagentSessionID:   params.SubagentSessionID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return taskcontract.Task{}, taskcontract.ErrTaskNotFound
	}
	if err != nil {
		return taskcontract.Task{}, err
	}
	return toTaskContract(row), nil
}

func completeTaskWithQuerier(ctx context.Context, q taskQuerier, params taskcontract.CompleteTaskParams) (taskcontract.Task, error) {
	row, err := q.CompleteTask(ctx, CompleteTaskParams{
		ResultPayloadJson: nullableString(params.ResultPayloadJSON),
		CompletedAt:       sql.NullInt64{Int64: params.CompletedAt.UTC().UnixMilli(), Valid: true},
		SubagentSessionID: params.SubagentSessionID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return taskcontract.Task{}, taskcontract.ErrTaskNotFound
	}
	if err != nil {
		return taskcontract.Task{}, err
	}
	return toTaskContract(row), nil
}

func failTaskWithQuerier(ctx context.Context, q taskQuerier, params taskcontract.FailTaskParams) (taskcontract.Task, error) {
	row, err := q.FailTask(ctx, FailTaskParams{
		ErrorMessage:      params.ErrorMessage,
		CompletedAt:       sql.NullInt64{Int64: params.CompletedAt.UTC().UnixMilli(), Valid: true},
		SubagentSessionID: params.SubagentSessionID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return taskcontract.Task{}, taskcontract.ErrTaskNotFound
	}
	if err != nil {
		return taskcontract.Task{}, err
	}
	return toTaskContract(row), nil
}

func toTaskContract(row Task) taskcontract.Task {
	var completedAt *time.Time
	if row.CompletedAt.Valid {
		ts := unixMilliToTime(row.CompletedAt.Int64)
		completedAt = &ts
	}

	return taskcontract.Task{
		SubagentSessionID:   row.SubagentSessionID,
		ParentSessionID:     row.ParentSessionID,
		Kind:                row.Kind,
		Status:              taskcontract.Status(row.Status),
		InputPayloadJSON:    row.InputPayloadJson.String,
		ProgressPayloadJSON: row.ProgressPayloadJson.String,
		ResultPayloadJSON:   row.ResultPayloadJson.String,
		ErrorMessage:        row.ErrorMessage,
		CreatedAt:           unixMilliToTime(row.CreatedAt),
		CompletedAt:         completedAt,
	}
}

func nullableString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}
