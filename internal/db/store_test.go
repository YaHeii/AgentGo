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
		ID:           "session-1",
		Title:        "first",
		CreatedAt:    firstAt,
		UpdatedAt:    firstAt,
		LastActiveAt: firstAt,
	})
	require.NoError(t, err)

	_, err = s.CreateSession(ctx, store.CreateSessionParams{
		ID:           "session-2",
		Title:        "second",
		CreatedAt:    secondAt,
		UpdatedAt:    secondAt,
		LastActiveAt: secondAt,
	})
	require.NoError(t, err)

	sessions, err := s.ListSessions(ctx)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	require.Equal(t, "session-2", sessions[0].ID)

	updated, err := s.UpdateSession(ctx, store.UpdateSessionParams{
		ID:           first.ID,
		Title:        "first renamed",
		UpdatedAt:    secondAt.Add(time.Minute),
		LastActiveAt: secondAt.Add(time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, "first renamed", updated.Title)

	loaded, err := s.GetSession(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, "first renamed", loaded.Title)

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
		ID:           "session-1",
		Title:        "chat",
		CreatedAt:    now,
		UpdatedAt:    now,
		LastActiveAt: now,
	})
	require.NoError(t, err)

	_, err = s.CreateMessage(ctx, store.CreateMessageParams{
		ID:        "message-1",
		SessionID: "session-1",
		Role:      "user",
		Content:   "hello",
		Status:    store.MessageStatusComplete,
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	assistant, err := s.CreateMessage(ctx, store.CreateMessageParams{
		ID:        "message-2",
		SessionID: "session-1",
		Role:      "assistant",
		Content:   "hel",
		Status:    store.MessageStatusStreaming,
		CreatedAt: now.Add(time.Second),
		UpdatedAt: now.Add(time.Second),
	})
	require.NoError(t, err)

	updated, err := s.UpdateMessage(ctx, store.UpdateMessageParams{
		ID:        assistant.ID,
		Content:   "hello back",
		Status:    store.MessageStatusComplete,
		UpdatedAt: now.Add(2 * time.Second),
	})
	require.NoError(t, err)
	require.Equal(t, store.MessageStatusComplete, updated.Status)

	messages, err := s.ListMessages(ctx, "session-1")
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "user", messages[0].Role)
	require.Equal(t, "hello back", messages[1].Content)

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
		ID:           "session-1",
		Title:        "draft-session",
		CreatedAt:    now,
		UpdatedAt:    now,
		LastActiveAt: now,
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
			ID:           "session-1",
			Title:        "tx-session",
			CreatedAt:    now,
			UpdatedAt:    now,
			LastActiveAt: now,
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
