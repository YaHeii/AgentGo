package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
	_ "modernc.org/sqlite/vec"

	"github.com/YaHeii/agentGo/internal/message"
	ragcontract "github.com/YaHeii/agentGo/internal/rag/contract"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
	taskcontract "github.com/YaHeii/agentGo/internal/task/contract"
)

type Store struct {
	db *sql.DB
	q  *Queries
}

type txStore struct {
	q *Queries
}

type TxStore interface {
	CreateSession(ctx context.Context, params sessioncontract.CreateSessionParams) (sessioncontract.Session, error)
	ListSessions(ctx context.Context) ([]sessioncontract.Session, error)
	GetSession(ctx context.Context, id string) (sessioncontract.Session, error)
	UpdateSession(ctx context.Context, params sessioncontract.UpdateSessionParams) (sessioncontract.Session, error)
	DeleteSession(ctx context.Context, id string) error

	CreateMessage(ctx context.Context, params message.CreateMessageRecordParams) (message.MessageRecord, error)
	ListMessages(ctx context.Context, sessionID string) ([]message.MessageRecord, error)
	GetMessage(ctx context.Context, id string) (message.MessageRecord, error)
	DeleteMessage(ctx context.Context, id string) error
	DeleteSessionMessages(ctx context.Context, sessionID string) error

	CreateTask(ctx context.Context, params taskcontract.CreateTaskParams) (taskcontract.Task, error)
	GetTask(ctx context.Context, subagentSessionID string) (taskcontract.Task, error)
	ListTasksByParentSession(ctx context.Context, parentSessionID string) ([]taskcontract.Task, error)
	UpdateTaskProgress(ctx context.Context, params taskcontract.UpdateTaskProgressParams) (taskcontract.Task, error)
	CompleteTask(ctx context.Context, params taskcontract.CompleteTaskParams) (taskcontract.Task, error)
	FailTask(ctx context.Context, params taskcontract.FailTaskParams) (taskcontract.Task, error)

	UpsertDocument(ctx context.Context, params ragcontract.UpsertDocumentParams) (ragcontract.Document, error)
	GetDocumentBySourcePath(ctx context.Context, sourcePath string) (ragcontract.Document, error)
	DeleteDocumentBySourcePath(ctx context.Context, sourcePath string) error
	CreateChunk(ctx context.Context, params ragcontract.CreateChunkParams) (ragcontract.Chunk, error)
	ListChunksByDocumentID(ctx context.Context, documentID int64) ([]ragcontract.Chunk, error)
	DeleteChunksByDocumentID(ctx context.Context, documentID int64) error
	SearchChunksByPrefix(ctx context.Context, params ragcontract.SearchChunksParams) ([]ragcontract.ChunkMatch, error)
}

type Transactor interface {
	WithinTx(ctx context.Context, fn func(tx TxStore) error) error
}

type dbStore interface {
	CreateSession(ctx context.Context, params sessioncontract.CreateSessionParams) (sessioncontract.Session, error)
	ListSessions(ctx context.Context) ([]sessioncontract.Session, error)
	GetSession(ctx context.Context, id string) (sessioncontract.Session, error)
	UpdateSession(ctx context.Context, params sessioncontract.UpdateSessionParams) (sessioncontract.Session, error)
	DeleteSession(ctx context.Context, id string) error

	CreateMessage(ctx context.Context, params message.CreateMessageRecordParams) (message.MessageRecord, error)
	ListMessages(ctx context.Context, sessionID string) ([]message.MessageRecord, error)
	GetMessage(ctx context.Context, id string) (message.MessageRecord, error)
	DeleteMessage(ctx context.Context, id string) error
	DeleteSessionMessages(ctx context.Context, sessionID string) error

	CreateTask(ctx context.Context, params taskcontract.CreateTaskParams) (taskcontract.Task, error)
	GetTask(ctx context.Context, subagentSessionID string) (taskcontract.Task, error)
	ListTasksByParentSession(ctx context.Context, parentSessionID string) ([]taskcontract.Task, error)
	UpdateTaskProgress(ctx context.Context, params taskcontract.UpdateTaskProgressParams) (taskcontract.Task, error)
	CompleteTask(ctx context.Context, params taskcontract.CompleteTaskParams) (taskcontract.Task, error)
	FailTask(ctx context.Context, params taskcontract.FailTaskParams) (taskcontract.Task, error)

	UpsertDocument(ctx context.Context, params ragcontract.UpsertDocumentParams) (ragcontract.Document, error)
	GetDocumentBySourcePath(ctx context.Context, sourcePath string) (ragcontract.Document, error)
	DeleteDocumentBySourcePath(ctx context.Context, sourcePath string) error
	CreateChunk(ctx context.Context, params ragcontract.CreateChunkParams) (ragcontract.Chunk, error)
	ListChunksByDocumentID(ctx context.Context, documentID int64) ([]ragcontract.Chunk, error)
	DeleteChunksByDocumentID(ctx context.Context, documentID int64) error
	SearchChunksByPrefix(ctx context.Context, params ragcontract.SearchChunksParams) ([]ragcontract.ChunkMatch, error)

	WithinTx(ctx context.Context, fn func(tx TxStore) error) error
	Close() error
}

var _ dbStore = (*Store)(nil)
var _ TxStore = (*txStore)(nil)

func Open(dbPath string) (*Store, error) {
	dbConn, err := openSQLite(dbPath)
	if err != nil {
		return nil, err
	}
	if err := migrateUp(context.Background(), dbConn, 0); err != nil {
		_ = dbConn.Close()
		return nil, err
	}

	return &Store{
		db: dbConn,
		q:  New(dbConn),
	}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) WithinTx(ctx context.Context, fn func(tx TxStore) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	wrapped := &txStore{q: s.q.WithTx(tx)}
	if err := fn(wrapped); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func openSQLite(dbPath string) (*sql.DB, error) {
	dbConn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	dbConn.SetMaxOpenConns(1)
	return dbConn, nil
}

func enableForeignKeys(ctx context.Context, dbConn *sql.DB) error {
	if _, err := dbConn.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	return nil
}

func unixMilliToTime(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}
