package task

import (
	"context"
	"testing"
	"time"

	taskcontract "github.com/YaHeii/agentGo/internal/task/contract"
	"github.com/stretchr/testify/require"
)

func TestTaskServiceCreateDelegatesToStoreAndFillsCreatedAt(t *testing.T) {
	t.Parallel()

	now := time.Unix(1710006000, 0).UTC()
	st := newFakeTaskStore()
	svc := NewTaskService(st)
	svc.nowFunc = func() time.Time { return now }

	created, err := svc.Create(context.Background(), taskcontract.CreateTaskParams{
		SubagentSessionID: "session-subagent",
		ParentSessionID:   "session-parent",
		Kind:              "verify",
		InputPayloadJSON:  `{"tasks":["a","b"]}`,
	})
	require.NoError(t, err)
	require.Equal(t, "session-subagent", created.SubagentSessionID)
	require.Equal(t, now, created.CreatedAt)

	require.NotNil(t, st.lastCreate)
	require.Equal(t, now, st.lastCreate.CreatedAt)
	require.Equal(t, `{"tasks":["a","b"]}`, st.lastCreate.InputPayloadJSON)
}

func TestTaskServiceGetDelegatesToStore(t *testing.T) {
	t.Parallel()

	st := newFakeTaskStore()
	st.getResult = taskcontract.Task{
		SubagentSessionID: "session-subagent",
		ParentSessionID:   "session-parent",
		Kind:              "verify",
		Status:            taskcontract.StatusRunning,
	}
	svc := NewTaskService(st)

	got, err := svc.Get(context.Background(), "session-subagent")
	require.NoError(t, err)
	require.Equal(t, st.getResult, got)
	require.Equal(t, "session-subagent", st.lastGetSubagentSessionID)
}

func TestTaskServiceListByParentSessionDelegatesToStore(t *testing.T) {
	t.Parallel()

	st := newFakeTaskStore()
	st.listResult = []taskcontract.Task{
		{SubagentSessionID: "session-subagent-1"},
		{SubagentSessionID: "session-subagent-2"},
	}
	svc := NewTaskService(st)

	got, err := svc.ListByParentSession(context.Background(), "session-parent")
	require.NoError(t, err)
	require.Equal(t, st.listResult, got)
	require.Equal(t, "session-parent", st.lastListParentSessionID)
}

func TestTaskServiceUpdateProgressDelegatesToStore(t *testing.T) {
	t.Parallel()

	st := newFakeTaskStore()
	st.updateProgressResult = taskcontract.Task{
		SubagentSessionID:   "session-subagent",
		ProgressPayloadJSON: `{"done":["a"]}`,
		Status:              taskcontract.StatusRunning,
	}
	svc := NewTaskService(st)

	got, err := svc.UpdateProgress(context.Background(), taskcontract.UpdateTaskProgressParams{
		SubagentSessionID:   "session-subagent",
		ProgressPayloadJSON: `{"done":["a"]}`,
	})
	require.NoError(t, err)
	require.Equal(t, st.updateProgressResult, got)
	require.NotNil(t, st.lastUpdateProgress)
	require.Equal(t, `{"done":["a"]}`, st.lastUpdateProgress.ProgressPayloadJSON)
}

func TestTaskServiceCompleteDelegatesToStoreAndFillsCompletedAt(t *testing.T) {
	t.Parallel()

	now := time.Unix(1710007000, 0).UTC()
	st := newFakeTaskStore()
	st.completeResult = taskcontract.Task{
		SubagentSessionID: "session-subagent",
		Status:            taskcontract.StatusComplete,
		CompletedAt:       timePtr(now),
	}
	svc := NewTaskService(st)
	svc.nowFunc = func() time.Time { return now }

	got, err := svc.Complete(context.Background(), taskcontract.CompleteTaskParams{
		SubagentSessionID: "session-subagent",
		ResultPayloadJSON: `{"summary":"ok"}`,
	})
	require.NoError(t, err)
	require.Equal(t, st.completeResult, got)
	require.NotNil(t, st.lastComplete)
	require.Equal(t, now, st.lastComplete.CompletedAt)
	require.Equal(t, `{"summary":"ok"}`, st.lastComplete.ResultPayloadJSON)
}

func TestTaskServiceFailDelegatesToStoreAndFillsCompletedAt(t *testing.T) {
	t.Parallel()

	now := time.Unix(1710008000, 0).UTC()
	st := newFakeTaskStore()
	st.failResult = taskcontract.Task{
		SubagentSessionID: "session-subagent",
		Status:            taskcontract.StatusFailed,
		CompletedAt:       timePtr(now),
		ErrorMessage:      "tool failed",
	}
	svc := NewTaskService(st)
	svc.nowFunc = func() time.Time { return now }

	got, err := svc.Fail(context.Background(), taskcontract.FailTaskParams{
		SubagentSessionID: "session-subagent",
		ErrorMessage:      "tool failed",
	})
	require.NoError(t, err)
	require.Equal(t, st.failResult, got)
	require.NotNil(t, st.lastFail)
	require.Equal(t, now, st.lastFail.CompletedAt)
	require.Equal(t, "tool failed", st.lastFail.ErrorMessage)
}

type fakeTaskStore struct {
	createResult             taskcontract.Task
	getResult                taskcontract.Task
	listResult               []taskcontract.Task
	updateProgressResult     taskcontract.Task
	completeResult           taskcontract.Task
	failResult               taskcontract.Task
	lastCreate               *taskcontract.CreateTaskParams
	lastGetSubagentSessionID string
	lastListParentSessionID  string
	lastUpdateProgress       *taskcontract.UpdateTaskProgressParams
	lastComplete             *taskcontract.CompleteTaskParams
	lastFail                 *taskcontract.FailTaskParams
}

func newFakeTaskStore() *fakeTaskStore {
	return &fakeTaskStore{}
}

func (s *fakeTaskStore) CreateTask(_ context.Context, params taskcontract.CreateTaskParams) (taskcontract.Task, error) {
	s.lastCreate = &params
	if s.createResult.SubagentSessionID == "" {
		s.createResult = taskcontract.Task{
			SubagentSessionID:   params.SubagentSessionID,
			ParentSessionID:     params.ParentSessionID,
			Kind:                params.Kind,
			Status:              taskcontract.StatusRunning,
			InputPayloadJSON:    params.InputPayloadJSON,
			ProgressPayloadJSON: params.ProgressPayloadJSON,
			ResultPayloadJSON:   params.ResultPayloadJSON,
			ErrorMessage:        params.ErrorMessage,
			CreatedAt:           params.CreatedAt,
		}
	}
	return s.createResult, nil
}

func (s *fakeTaskStore) GetTask(_ context.Context, subagentSessionID string) (taskcontract.Task, error) {
	s.lastGetSubagentSessionID = subagentSessionID
	return s.getResult, nil
}

func (s *fakeTaskStore) ListTasksByParentSession(_ context.Context, parentSessionID string) ([]taskcontract.Task, error) {
	s.lastListParentSessionID = parentSessionID
	return append([]taskcontract.Task(nil), s.listResult...), nil
}

func (s *fakeTaskStore) UpdateTaskProgress(_ context.Context, params taskcontract.UpdateTaskProgressParams) (taskcontract.Task, error) {
	s.lastUpdateProgress = &params
	return s.updateProgressResult, nil
}

func (s *fakeTaskStore) CompleteTask(_ context.Context, params taskcontract.CompleteTaskParams) (taskcontract.Task, error) {
	s.lastComplete = &params
	return s.completeResult, nil
}

func (s *fakeTaskStore) FailTask(_ context.Context, params taskcontract.FailTaskParams) (taskcontract.Task, error) {
	s.lastFail = &params
	return s.failResult, nil
}

func timePtr(v time.Time) *time.Time {
	return &v
}
