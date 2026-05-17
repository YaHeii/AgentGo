package tool

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"github.com/stretchr/testify/require"
)

func TestTodosExecuteRequiresSessionID(t *testing.T) {
	t.Parallel()

	tool := NewTodosTool(&stubTodosSessionStore{})
	args := json.RawMessage(`{"todos":[{"content":"write tests","status":"pending","active_form":"Writing tests"}]}`)

	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID:      "call-1",
		Name:            TodosToolName,
		Arguments:       args,
		PermissionLevel: toolcontract.SafeLevel,
	})

	require.Equal(t, toolcontract.StatusValidationFailed, result.Status)
	require.Contains(t, result.Content, "session_id")
}

func TestTodosExecuteUpdatesSessionTodosJSON(t *testing.T) {
	t.Parallel()

	now := time.Unix(1710010000, 0).UTC()
	st := &stubTodosSessionStore{
		session: sessioncontract.Session{
			ID:        "session-1",
			Title:     "chat",
			TodosJSON: `[{"content":"write tests","status":"pending","active_form":"Writing tests"}]`,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	tool := NewTodosTool(st)

	args := json.RawMessage(`{
		"todos": [
			{"content":"write tests","status":"completed","active_form":"Writing tests"},
			{"content":"run tool tests","status":"in_progress","active_form":"Running tool tests"}
		]
	}`)

	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID:      "call-1",
		Name:            TodosToolName,
		Arguments:       args,
		PermissionLevel: toolcontract.SafeLevel,
		Context: toolcontract.ToolCallContext{
			SessionID: "session-1",
		},
	})

	require.Equal(t, toolcontract.StatusSuccess, result.Status)
	require.Contains(t, result.Content, "Todo list updated")
	require.JSONEq(t, `[
		{"content":"write tests","status":"completed","active_form":"Writing tests"},
		{"content":"run tool tests","status":"in_progress","active_form":"Running tool tests"}
	]`, st.saved.TodosJSON)
	require.Equal(t, "session-1", st.saved.ID)
	require.NotNil(t, result.Metadata)
	require.Equal(t, 1, result.Metadata["completed"])
	require.Equal(t, 2, result.Metadata["total"])
	require.Equal(t, "Running tool tests", result.Metadata["just_started"])
}

func TestTodosExecuteRejectsInvalidStatus(t *testing.T) {
	t.Parallel()

	now := time.Unix(1710010000, 0).UTC()
	st := &stubTodosSessionStore{
		session: sessioncontract.Session{
			ID:        "session-1",
			Title:     "chat",
			TodosJSON: `[]`,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	tool := NewTodosTool(st)

	args := json.RawMessage(`{"todos":[{"content":"bad","status":"paused","active_form":"Paused"}]}`)

	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID:      "call-1",
		Name:            TodosToolName,
		Arguments:       args,
		PermissionLevel: toolcontract.SafeLevel,
		Context: toolcontract.ToolCallContext{
			SessionID: "session-1",
		},
	})

	require.Equal(t, toolcontract.StatusValidationFailed, result.Status)
	require.Contains(t, result.Content, "invalid status")
}

type stubTodosSessionStore struct {
	session sessioncontract.Session
	saved   sessioncontract.Session
}

func (s *stubTodosSessionStore) Get(_ context.Context, id string) (sessioncontract.Session, error) {
	if s.session.ID != id {
		return sessioncontract.Session{}, sessioncontract.ErrSessionNotFound
	}
	return s.session, nil
}

func (s *stubTodosSessionStore) Save(_ context.Context, session sessioncontract.Session) (sessioncontract.Session, error) {
	s.saved = session
	s.session = session
	return session, nil
}
