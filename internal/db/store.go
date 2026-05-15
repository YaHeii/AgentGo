package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"time"

	_ "modernc.org/sqlite"

	"github.com/YaHeii/agentGo/internal/message"
	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

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
	dbConn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	dbConn.SetMaxOpenConns(1)

	if err := initSchema(context.Background(), dbConn); err != nil {
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

func initSchema(ctx context.Context, dbConn *sql.DB) error {
	if _, err := dbConn.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		sqlBytes, err := migrationsFS.ReadFile(path.Join("migrations", name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := dbConn.ExecContext(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}

	return nil
}

func unixMilliToTime(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}
