package db_test

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"

	db "github.com/YaHeii/agentGo/internal/db"
	"github.com/YaHeii/agentGo/internal/message"
	ragcontract "github.com/YaHeii/agentGo/internal/rag/contract"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
	taskcontract "github.com/YaHeii/agentGo/internal/task/contract"
	"github.com/stretchr/testify/require"
)

func TestStoreCreatesListsGetsUpdatesAndDeletesSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)

	firstAt := time.Unix(1710000000, 0).UTC()
	secondAt := firstAt.Add(2 * time.Minute)

	first, err := s.CreateSession(ctx, sessioncontract.CreateSessionParams{
		ID:               "session-1",
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

	_, err = s.CreateSession(ctx, sessioncontract.CreateSessionParams{
		ID:               "session-2",
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

	updated, err := s.UpdateSession(ctx, sessioncontract.UpdateSessionParams{
		ID:               first.ID,
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
	require.True(t, errors.Is(err, sessioncontract.ErrSessionNotFound))
}

func TestStoreCreatesListsUpdatesMessagesAndCascadesOnSessionDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)

	now := time.Unix(1710001000, 0).UTC()

	_, err := s.CreateSession(ctx, sessioncontract.CreateSessionParams{
		ID:        "session-1",
		Title:     "chat",
		TodosJSON: "[]",
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	_, err = s.CreateMessage(ctx, message.CreateMessageRecordParams{
		ID:               "message-1",
		SessionID:        "session-1",
		Kind:             "user",
		Provider:         "",
		FinishedAt:       now,
		IsCompactSummary: false,
		MessageJSON:      `{"flags":{},"parts":[{"Type":"text","Text":"hello"}]}`,
	})
	require.NoError(t, err)

	assistant, err := s.CreateMessage(ctx, message.CreateMessageRecordParams{
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

func TestOpenAppliesMigrationThatRemovesDraftsTable(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "agentgo.db")

	s, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, s.Close())
	})

	require.False(t, tableExists(t, dbPath, "drafts"))
}

func TestStoreWithinTxRollsBackOnError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)

	now := time.Unix(1710003000, 0).UTC()

	err := s.WithinTx(ctx, func(tx db.TxStore) error {
		_, err := tx.CreateSession(ctx, sessioncontract.CreateSessionParams{
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

func TestStoreCreatesGetsListsAndUpdatesTasks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	now := time.Unix(1710003500, 0).UTC()

	createTaskSessions(t, ctx, s, now, "session-parent", "session-subagent-1", "session-subagent-2")

	created, err := s.CreateTask(ctx, taskcontract.CreateTaskParams{
		SubagentSessionID:   "session-subagent-1",
		ParentSessionID:     "session-parent",
		Kind:                "verify",
		InputPayloadJSON:    `{"tasks":["a","b"]}`,
		ProgressPayloadJSON: `{"done":["a"]}`,
		CreatedAt:           now,
	})
	require.NoError(t, err)
	require.Equal(t, taskcontract.StatusRunning, created.Status)
	require.Equal(t, `{"tasks":["a","b"]}`, created.InputPayloadJSON)
	require.Equal(t, `{"done":["a"]}`, created.ProgressPayloadJSON)
	require.Empty(t, created.ResultPayloadJSON)
	require.Empty(t, created.ErrorMessage)
	require.Nil(t, created.CompletedAt)

	_, err = s.CreateTask(ctx, taskcontract.CreateTaskParams{
		SubagentSessionID: "session-subagent-2",
		ParentSessionID:   "session-parent",
		Kind:              "verify",
		CreatedAt:         now.Add(time.Second),
	})
	require.NoError(t, err)

	loaded, err := s.GetTask(ctx, "session-subagent-1")
	require.NoError(t, err)
	require.Equal(t, created, loaded)

	tasks, err := s.ListTasksByParentSession(ctx, "session-parent")
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	require.Equal(t, "session-subagent-1", tasks[0].SubagentSessionID)
	require.Equal(t, "session-subagent-2", tasks[1].SubagentSessionID)

	progressed, err := s.UpdateTaskProgress(ctx, taskcontract.UpdateTaskProgressParams{
		SubagentSessionID:   "session-subagent-1",
		ProgressPayloadJSON: `{"done":["a","b"]}`,
	})
	require.NoError(t, err)
	require.Equal(t, taskcontract.StatusRunning, progressed.Status)
	require.Equal(t, `{"done":["a","b"]}`, progressed.ProgressPayloadJSON)
	require.Nil(t, progressed.CompletedAt)
}

func TestStoreCompleteTaskPersistsResultAndCompletedAt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	now := time.Unix(1710003600, 0).UTC()

	createTaskSessions(t, ctx, s, now, "session-parent", "session-subagent")

	_, err := s.CreateTask(ctx, taskcontract.CreateTaskParams{
		SubagentSessionID: "session-subagent",
		ParentSessionID:   "session-parent",
		Kind:              "verify",
		CreatedAt:         now,
	})
	require.NoError(t, err)

	completedAt := now.Add(2 * time.Minute)
	completed, err := s.CompleteTask(ctx, taskcontract.CompleteTaskParams{
		SubagentSessionID: "session-subagent",
		ResultPayloadJSON: `{"summary":"ok"}`,
		CompletedAt:       completedAt,
	})
	require.NoError(t, err)
	require.Equal(t, taskcontract.StatusComplete, completed.Status)
	require.Equal(t, `{"summary":"ok"}`, completed.ResultPayloadJSON)
	require.NotNil(t, completed.CompletedAt)
	require.Equal(t, completedAt, *completed.CompletedAt)
	require.Empty(t, completed.ErrorMessage)
}

func TestStoreFailTaskPersistsErrorAndCompletedAt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	now := time.Unix(1710003700, 0).UTC()

	createTaskSessions(t, ctx, s, now, "session-parent", "session-subagent")

	_, err := s.CreateTask(ctx, taskcontract.CreateTaskParams{
		SubagentSessionID: "session-subagent",
		ParentSessionID:   "session-parent",
		Kind:              "verify",
		CreatedAt:         now,
	})
	require.NoError(t, err)

	completedAt := now.Add(3 * time.Minute)
	failed, err := s.FailTask(ctx, taskcontract.FailTaskParams{
		SubagentSessionID: "session-subagent",
		ErrorMessage:      "tool failed",
		CompletedAt:       completedAt,
	})
	require.NoError(t, err)
	require.Equal(t, taskcontract.StatusFailed, failed.Status)
	require.Equal(t, "tool failed", failed.ErrorMessage)
	require.NotNil(t, failed.CompletedAt)
	require.Equal(t, completedAt, *failed.CompletedAt)
	require.Empty(t, failed.ResultPayloadJSON)
}

func TestStoreWithinTxRollsBackCreatedSubagentSessionAndTask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	now := time.Unix(1710003800, 0).UTC()

	_, err := s.CreateSession(ctx, sessioncontract.CreateSessionParams{
		ID:        "session-parent",
		Title:     "parent",
		TodosJSON: "[]",
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	err = s.WithinTx(ctx, func(tx db.TxStore) error {
		_, err := tx.CreateSession(ctx, sessioncontract.CreateSessionParams{
			ID:        "session-subagent",
			Title:     "subagent",
			TodosJSON: "[]",
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)

		_, err = tx.CreateTask(ctx, taskcontract.CreateTaskParams{
			SubagentSessionID: "session-subagent",
			ParentSessionID:   "session-parent",
			Kind:              "verify",
			CreatedAt:         now,
		})
		require.NoError(t, err)

		return errors.New("force rollback")
	})
	require.EqualError(t, err, "force rollback")

	_, err = s.GetSession(ctx, "session-subagent")
	require.ErrorIs(t, err, sessioncontract.ErrSessionNotFound)

	_, err = s.GetTask(ctx, "session-subagent")
	require.ErrorIs(t, err, taskcontract.ErrTaskNotFound)
}

func TestOpenPreservesSessionsAcrossReopen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "agentgo.db")
	now := time.Unix(1710004000, 0).UTC()

	first, err := db.Open(dbPath)
	require.NoError(t, err)

	_, err = first.CreateSession(ctx, sessioncontract.CreateSessionParams{
		ID:        "session-1",
		Title:     "persisted",
		TodosJSON: "[]",
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)
	require.NoError(t, first.Close())

	second, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, second.Close())
	})

	sessions, err := second.ListSessions(ctx)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, "session-1", sessions[0].ID)
}

func TestMigrateDownRollsBackLatestMigration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "agentgo.db")

	store, err := db.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, store.Close())

	require.NoError(t, db.MigrateDown(ctx, dbPath, 1))
	require.True(t, tableExists(t, dbPath, "sessions"))
	require.False(t, tableExists(t, dbPath, "documents"))
	require.False(t, tableExists(t, dbPath, "chunks"))

	reopened, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, reopened.Close())
	})

	require.True(t, tableExists(t, dbPath, "sessions"))
	require.True(t, tableExists(t, dbPath, "documents"))
	require.True(t, tableExists(t, dbPath, "chunks"))
	require.False(t, tableExists(t, dbPath, "drafts"))
}

func TestOpenAppliesRAGSchemaMigration(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "agentgo.db")

	s, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, s.Close())
	})

	require.True(t, tableExists(t, dbPath, "documents"))
	require.True(t, tableExists(t, dbPath, "chunks"))
	require.True(t, indexExists(t, dbPath, "documents_normalized_path_idx"))
}

func TestOpenAppliesTaskSchemaMigration(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "agentgo.db")

	s, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, s.Close())
	})

	require.True(t, tableExists(t, dbPath, "tasks"))
	require.True(t, indexExists(t, dbPath, "tasks_parent_session_status_idx"))
	require.True(t, indexExists(t, dbPath, "tasks_kind_status_idx"))
	require.False(t, columnExists(t, dbPath, "sessions", "parent_session_id"))
}

func TestStoreDeletingSubagentSessionCascadesTask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "agentgo.db")
	s, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, s.Close())
	})
	now := time.Unix(1710005000, 0).UTC()

	_, err = s.CreateSession(ctx, sessioncontract.CreateSessionParams{
		ID:        "session-parent",
		Title:     "parent",
		TodosJSON: "[]",
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	_, err = s.CreateSession(ctx, sessioncontract.CreateSessionParams{
		ID:        "session-subagent",
		Title:     "subagent",
		TodosJSON: "[]",
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	_, err = conn.ExecContext(ctx, `
INSERT INTO tasks (
    subagent_session_id,
    parent_session_id,
    kind,
    status,
    input_payload_json,
    progress_payload_json,
    result_payload_json,
    error_message,
    created_at,
    completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, "session-subagent", "session-parent", "verify", "running", "{}", "{}", "", "", now.UnixMilli(), nil)
	require.NoError(t, err)

	require.NoError(t, s.DeleteSession(ctx, "session-subagent"))
	require.Equal(t, 0, taskCountBySession(t, conn, "session-subagent"))
}

func TestStoreDeletingParentSessionAlsoDeletesSubagentSessionsAndTasks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	now := time.Unix(1710005200, 0).UTC()

	createTaskSessions(t, ctx, s, now, "session-parent", "session-subagent-1", "session-subagent-2")

	_, err := s.CreateTask(ctx, taskcontract.CreateTaskParams{
		SubagentSessionID: "session-subagent-1",
		ParentSessionID:   "session-parent",
		Kind:              "verify",
		CreatedAt:         now,
	})
	require.NoError(t, err)

	_, err = s.CreateTask(ctx, taskcontract.CreateTaskParams{
		SubagentSessionID: "session-subagent-2",
		ParentSessionID:   "session-parent",
		Kind:              "verify",
		CreatedAt:         now.Add(time.Second),
	})
	require.NoError(t, err)

	require.NoError(t, s.DeleteSession(ctx, "session-parent"))

	_, err = s.GetSession(ctx, "session-parent")
	require.ErrorIs(t, err, sessioncontract.ErrSessionNotFound)
	_, err = s.GetSession(ctx, "session-subagent-1")
	require.ErrorIs(t, err, sessioncontract.ErrSessionNotFound)
	_, err = s.GetSession(ctx, "session-subagent-2")
	require.ErrorIs(t, err, sessioncontract.ErrSessionNotFound)
	_, err = s.GetTask(ctx, "session-subagent-1")
	require.ErrorIs(t, err, taskcontract.ErrTaskNotFound)
	_, err = s.GetTask(ctx, "session-subagent-2")
	require.ErrorIs(t, err, taskcontract.ErrTaskNotFound)
}

func TestOpenRegistersVecFunctions(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "agentgo.db")

	s, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, s.Close())
	})

	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	var version string
	err = conn.QueryRow(`SELECT vec_version()`).Scan(&version)
	require.NoError(t, err)
	require.NotEmpty(t, version)
}

func TestStoreUpsertsDocuments(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)

	firstAt := time.Unix(1710010000, 0).UTC()
	secondAt := firstAt.Add(5 * time.Minute)

	first, err := s.UpsertDocument(ctx, ragcontract.UpsertDocumentParams{
		SourcePath:     "docs/guide.md",
		NormalizedPath: "docs/guide.md",
		FileHash:       "hash-1",
		UpdatedAt:      firstAt,
	})
	require.NoError(t, err)
	require.NotZero(t, first.ID)
	require.Equal(t, "hash-1", first.FileHash)

	second, err := s.UpsertDocument(ctx, ragcontract.UpsertDocumentParams{
		SourcePath:     "docs/guide.md",
		NormalizedPath: "docs/guide.md",
		FileHash:       "hash-2",
		UpdatedAt:      secondAt,
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, "hash-2", second.FileHash)
	require.Equal(t, secondAt, second.UpdatedAt)

	loaded, err := s.GetDocumentBySourcePath(ctx, "docs/guide.md")
	require.NoError(t, err)
	require.Equal(t, second.ID, loaded.ID)
	require.Equal(t, "docs/guide.md", loaded.NormalizedPath)
	require.Equal(t, "hash-2", loaded.FileHash)
}

func TestStoreGetDocumentBySourcePathReturnsNotFound(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)

	_, err := s.GetDocumentBySourcePath(context.Background(), "missing.md")
	require.ErrorIs(t, err, ragcontract.ErrDocumentNotFound)
}

func TestStoreCreatesListsAndDeletesChunks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	now := time.Unix(1710011000, 0).UTC()

	doc, err := s.UpsertDocument(ctx, ragcontract.UpsertDocumentParams{
		SourcePath:     "docs/a.md",
		NormalizedPath: "docs/a.md",
		FileHash:       "hash-a",
		UpdatedAt:      now,
	})
	require.NoError(t, err)

	firstChunk, err := s.CreateChunk(ctx, ragcontract.CreateChunkParams{
		DocumentID: doc.ID,
		ChunkIndex: 0,
		Content:    "chunk 0",
		Embedding:  testEmbedding(0.01),
	})
	require.NoError(t, err)
	require.NotZero(t, firstChunk.ID)

	_, err = s.CreateChunk(ctx, ragcontract.CreateChunkParams{
		DocumentID: doc.ID,
		ChunkIndex: 1,
		Content:    "chunk 1",
		Embedding:  testEmbedding(0.02),
	})
	require.NoError(t, err)

	chunks, err := s.ListChunksByDocumentID(ctx, doc.ID)
	require.NoError(t, err)
	require.Len(t, chunks, 2)
	require.Equal(t, int64(0), chunks[0].ChunkIndex)
	require.Equal(t, int64(1), chunks[1].ChunkIndex)

	require.NoError(t, s.DeleteChunksByDocumentID(ctx, doc.ID))

	chunks, err = s.ListChunksByDocumentID(ctx, doc.ID)
	require.NoError(t, err)
	require.Len(t, chunks, 0)
}

func TestStoreRejectsDuplicateChunkIndexPerDocument(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	now := time.Unix(1710012000, 0).UTC()

	doc, err := s.UpsertDocument(ctx, ragcontract.UpsertDocumentParams{
		SourcePath:     "docs/dup.md",
		NormalizedPath: "docs/dup.md",
		FileHash:       "hash-dup",
		UpdatedAt:      now,
	})
	require.NoError(t, err)

	_, err = s.CreateChunk(ctx, ragcontract.CreateChunkParams{
		DocumentID: doc.ID,
		ChunkIndex: 0,
		Content:    "chunk",
		Embedding:  testEmbedding(0.03),
	})
	require.NoError(t, err)

	_, err = s.CreateChunk(ctx, ragcontract.CreateChunkParams{
		DocumentID: doc.ID,
		ChunkIndex: 0,
		Content:    "chunk duplicate",
		Embedding:  testEmbedding(0.04),
	})
	require.Error(t, err)
}

func TestStoreRejectsEmbeddingWithWrongDimension(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	now := time.Unix(1710013000, 0).UTC()

	doc, err := s.UpsertDocument(ctx, ragcontract.UpsertDocumentParams{
		SourcePath:     "docs/wrong-dim.md",
		NormalizedPath: "docs/wrong-dim.md",
		FileHash:       "hash-wrong-dim",
		UpdatedAt:      now,
	})
	require.NoError(t, err)

	_, err = s.CreateChunk(ctx, ragcontract.CreateChunkParams{
		DocumentID: doc.ID,
		ChunkIndex: 0,
		Content:    "bad embedding",
		Embedding:  []byte{1, 2, 3, 4},
	})
	require.Error(t, err)
}

func TestStoreDeleteDocumentCascadesChunks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	now := time.Unix(1710014000, 0).UTC()

	doc, err := s.UpsertDocument(ctx, ragcontract.UpsertDocumentParams{
		SourcePath:     "docs/cascade.md",
		NormalizedPath: "docs/cascade.md",
		FileHash:       "hash-cascade",
		UpdatedAt:      now,
	})
	require.NoError(t, err)

	_, err = s.CreateChunk(ctx, ragcontract.CreateChunkParams{
		DocumentID: doc.ID,
		ChunkIndex: 0,
		Content:    "chunk 0",
		Embedding:  testEmbedding(0.05),
	})
	require.NoError(t, err)

	require.NoError(t, s.DeleteDocumentBySourcePath(ctx, "docs/cascade.md"))

	chunks, err := s.ListChunksByDocumentID(ctx, doc.ID)
	require.NoError(t, err)
	require.Len(t, chunks, 0)

	_, err = s.GetDocumentBySourcePath(ctx, "docs/cascade.md")
	require.ErrorIs(t, err, ragcontract.ErrDocumentNotFound)
}

func TestStoreSearchChunksByPrefixFiltersAndOrdersByDistance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	now := time.Unix(1710015000, 0).UTC()

	matchingDoc, err := s.UpsertDocument(ctx, ragcontract.UpsertDocumentParams{
		SourcePath:     "docs/rag/guide.md",
		NormalizedPath: "docs/rag/guide.md",
		FileHash:       "hash-guide",
		UpdatedAt:      now,
	})
	require.NoError(t, err)

	otherDoc, err := s.UpsertDocument(ctx, ragcontract.UpsertDocumentParams{
		SourcePath:     "docs/other/guide.md",
		NormalizedPath: "docs/other/guide.md",
		FileHash:       "hash-other",
		UpdatedAt:      now,
	})
	require.NoError(t, err)

	near, err := s.CreateChunk(ctx, ragcontract.CreateChunkParams{
		DocumentID: matchingDoc.ID,
		ChunkIndex: 0,
		Content:    "near",
		Embedding:  oneHotEmbedding(0),
	})
	require.NoError(t, err)

	far, err := s.CreateChunk(ctx, ragcontract.CreateChunkParams{
		DocumentID: matchingDoc.ID,
		ChunkIndex: 1,
		Content:    "far",
		Embedding:  oneHotEmbedding(1),
	})
	require.NoError(t, err)

	_, err = s.CreateChunk(ctx, ragcontract.CreateChunkParams{
		DocumentID: otherDoc.ID,
		ChunkIndex: 0,
		Content:    "outside prefix",
		Embedding:  oneHotEmbedding(0),
	})
	require.NoError(t, err)

	matches, err := s.SearchChunksByPrefix(ctx, ragcontract.SearchChunksParams{
		NormalizedPathGlob: "docs/rag/*",
		QueryEmbedding:     oneHotEmbedding(0),
		TopK:               5,
	})
	require.NoError(t, err)
	require.Len(t, matches, 2)
	require.Equal(t, near.ID, matches[0].Chunk.ID)
	require.Equal(t, far.ID, matches[1].Chunk.ID)
	require.LessOrEqual(t, matches[0].Distance, matches[1].Distance)
	for _, match := range matches {
		require.Equal(t, matchingDoc.ID, match.Document.ID)
		require.Contains(t, match.Document.NormalizedPath, "docs/rag/")
	}
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

func createTaskSessions(t *testing.T, ctx context.Context, s *db.Store, now time.Time, parentSessionID string, subagentSessionIDs ...string) {
	t.Helper()

	_, err := s.CreateSession(ctx, sessioncontract.CreateSessionParams{
		ID:        parentSessionID,
		Title:     "parent",
		TodosJSON: "[]",
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	for _, subagentSessionID := range subagentSessionIDs {
		_, err := s.CreateSession(ctx, sessioncontract.CreateSessionParams{
			ID:        subagentSessionID,
			Title:     "subagent",
			TodosJSON: "[]",
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)
	}
}

func tableExists(t *testing.T, dbPath string, tableName string) bool {
	t.Helper()

	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	var count int
	err = conn.QueryRow(
		`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		tableName,
	).Scan(&count)
	require.NoError(t, err)
	return count == 1
}

func indexExists(t *testing.T, dbPath string, indexName string) bool {
	t.Helper()

	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	var count int
	err = conn.QueryRow(
		`SELECT COUNT(1) FROM sqlite_master WHERE type = 'index' AND name = ?`,
		indexName,
	).Scan(&count)
	require.NoError(t, err)
	return count == 1
}

func columnExists(t *testing.T, dbPath string, tableName string, columnName string) bool {
	t.Helper()

	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	rows, err := conn.Query(`PRAGMA table_info(` + tableName + `)`)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, rows.Close())
	}()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		require.NoError(t, rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey))
		if name == columnName {
			return true
		}
	}
	require.NoError(t, rows.Err())
	return false
}

func taskCountBySession(t *testing.T, conn *sql.DB, subagentSessionID string) int {
	t.Helper()

	var count int
	err := conn.QueryRowContext(
		context.Background(),
		`SELECT COUNT(1) FROM tasks WHERE subagent_session_id = ?`,
		subagentSessionID,
	).Scan(&count)
	require.NoError(t, err)
	return count
}

func testEmbedding(value float32) []byte {
	buf := make([]byte, 1024*4)
	for i := 0; i < 1024; i++ {
		bits := math.Float32bits(value)
		offset := i * 4
		buf[offset] = byte(bits)
		buf[offset+1] = byte(bits >> 8)
		buf[offset+2] = byte(bits >> 16)
		buf[offset+3] = byte(bits >> 24)
	}
	return buf
}

func oneHotEmbedding(index int) []byte {
	buf := make([]byte, 1024*4)
	bits := math.Float32bits(1)
	offset := index * 4
	buf[offset] = byte(bits)
	buf[offset+1] = byte(bits >> 8)
	buf[offset+2] = byte(bits >> 16)
	buf[offset+3] = byte(bits >> 24)
	return buf
}
