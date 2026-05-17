package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/YaHeii/agentGo/internal/message"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
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

	LoadDraft(ctx context.Context, sessionID string) (string, error)
	SaveDraft(ctx context.Context, sessionID string, content string, updatedAt time.Time) error
	DeleteDraft(ctx context.Context, sessionID string) error
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

	LoadDraft(ctx context.Context, sessionID string) (string, error)
	SaveDraft(ctx context.Context, sessionID string, content string, updatedAt time.Time) error
	DeleteDraft(ctx context.Context, sessionID string) error

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
