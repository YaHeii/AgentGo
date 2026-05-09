package db_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	db "github.com/YaHeii/agentGo/internal/db"
	"github.com/YaHeii/agentGo/internal/store"
	"github.com/stretchr/testify/require"
)

func TestStoreCreatesListsGetsUpdatesAndDeletesSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)

	firstAt := time.Unix(1710000000, 0).UTC()
	secondAt := firstAt.Add(2 * time.Minute)

	first, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:               "session-1",
		ParentSessionID:  "",
		Title:            "first",
		MessageCount:     1,
		CompletionTokens: 12,
		CostMicros:       34,
		SummaryMessageID: "",
		TodosJSON:        "[]",
		CreatedAt:        firstAt,
		UpdatedAt:        firstAt,
	})
	require.NoError(t, err)

	_, err = s.CreateSession(ctx, store.CreateSessionParams{
		ID:               "session-2",
		ParentSessionID:  "session-1",
		Title:            "second",
		MessageCount:     2,
		CompletionTokens: 99,
		CostMicros:       123,
		SummaryMessageID: "summary-1",
		TodosJSON:        `[{"content":"ship","status":"pending"}]`,
		CreatedAt:        secondAt,
		UpdatedAt:        secondAt,
	})
	require.NoError(t, err)

	sessions, err := s.ListSessions(ctx)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, "session-2", sessions[0].ID)
	require.Equal(t, int64(99), sessions[0].CompletionTokens)
	require.Equal(t, int64(123), sessions[0].CostMicros)
	require.Equal(t, "summary-1", sessions[0].SummaryMessageID)

	updated, err := s.UpdateSession(ctx, store.UpdateSessionParams{
		ID:               first.ID,
		ParentSessionID:  "",
		Title:            "first renamed",
		MessageCount:     3,
		CompletionTokens: 55,
		CostMicros:       89,
		SummaryMessageID: "summary-2",
		TodosJSON:        `[{"content":"done","status":"completed"}]`,
		UpdatedAt:        secondAt.Add(time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, "first renamed", updated.Title)
	require.Equal(t, int64(3), updated.MessageCount)
	require.Equal(t, int64(55), updated.CompletionTokens)
	require.Equal(t, int64(89), updated.CostMicros)

	loaded, err := s.GetSession(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, "first renamed", loaded.Title)
	require.Equal(t, "summary-2", loaded.SummaryMessageID)

	require.NoError(t, s.DeleteSession(ctx, "session-2"))

	_, err = s.GetSession(ctx, "session-2")
	require.True(t, errors.Is(err, store.ErrSessionNotFound))
}

func TestStoreCreatesListsUpdatesMessagesAndCascadesOnSessionDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)

	now := time.Unix(1710001000, 0).UTC()

	_, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:        "session-1",
		Title:     "chat",
		TodosJSON: "[]",
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	_, err = s.CreateMessage(ctx, store.CreateMessageParams{
		ID:               "message-1",
		SessionID:        "session-1",
		Kind:             "user",
		Provider:         "",
		FinishedAt:       now,
		IsCompactSummary: false,
		MessageJSON:      `{"flags":{},"parts":[{"Type":"text","Text":"hello"}]}`,
	})
	require.NoError(t, err)

	assistant, err := s.CreateMessage(ctx, store.CreateMessageParams{
		ID:               "message-2",
		SessionID:        "session-1",
		Kind:             "assistant",
		Provider:         "openai/gpt-4.1",
		FinishedAt:       now.Add(time.Second),
		IsCompactSummary: false,
		MessageJSON:      `{"flags":{},"parts":[{"Type":"text","Text":"hello back"}]}`,
	})
	require.NoError(t, err)
	require.Equal(t, "openai/gpt-4.1", assistant.Provider)

	messages, err := s.ListMessages(ctx, "session-1")
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "user", messages[0].Kind)
	require.Equal(t, "assistant", messages[1].Kind)
	require.Equal(t, `{"flags":{},"parts":[{"Type":"text","Text":"hello back"}]}`, messages[1].MessageJSON)

	require.NoError(t, s.DeleteSession(ctx, "session-1"))

	messages, err = s.ListMessages(ctx, "session-1")
	require.NoError(t, err)
	require.Len(t, messages, 0)
}

func TestStoreLoadsSavesAndDeletesDrafts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)

	now := time.Unix(1710002000, 0).UTC()

	_, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:        "session-1",
		Title:     "draft-session",
		TodosJSON: "[]",
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	draft, err := s.LoadDraft(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, "", draft)

	err = s.SaveDraft(ctx, store.SaveDraftParams{
		SessionID: "session-1",
		Content:   "/session",
		UpdatedAt: now.Add(time.Second),
	})
	require.NoError(t, err)

	draft, err = s.LoadDraft(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, "/session", draft)

	err = s.SaveDraft(ctx, store.SaveDraftParams{
		SessionID: "session-1",
		Content:   "/session list",
		UpdatedAt: now.Add(2 * time.Second),
	})
	require.NoError(t, err)

	draft, err = s.LoadDraft(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, "/session list", draft)

	require.NoError(t, s.DeleteDraft(ctx, "session-1"))

	draft, err = s.LoadDraft(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, "", draft)
}

func TestStoreWithinTxRollsBackOnError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)

	now := time.Unix(1710003000, 0).UTC()

	err := s.WithinTx(ctx, func(tx store.TxStore) error {
		_, err := tx.CreateSession(ctx, store.CreateSessionParams{
			ID:        "session-1",
			Title:     "tx-session",
			TodosJSON: "[]",
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)

		return errors.New("force rollback")
	})
	require.EqualError(t, err, "force rollback")

	sessions, err := s.ListSessions(ctx)
	require.NoError(t, err)
	require.Len(t, sessions, 0)
}

func newTestStore(t *testing.T) *db.Store {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "agentgo.db")
	s, err := db.Open(dbPath)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, s.Close())
	})

	return s
}
