package store

import "context"

type TxStore interface {
	CreateSession(ctx context.Context, params CreateSessionParams) (Session, error)
	ListSessions(ctx context.Context) ([]Session, error)
	GetSession(ctx context.Context, id string) (Session, error)
	UpdateSession(ctx context.Context, params UpdateSessionParams) (Session, error)
	DeleteSession(ctx context.Context, id string) error

	CreateMessage(ctx context.Context, params CreateMessageParams) (Message, error)
	ListMessages(ctx context.Context, sessionID string) ([]Message, error)

	LoadDraft(ctx context.Context, sessionID string) (string, error)
	SaveDraft(ctx context.Context, params SaveDraftParams) error
	DeleteDraft(ctx context.Context, sessionID string) error
}

type Transactor interface {
	WithinTx(ctx context.Context, fn func(tx TxStore) error) error
}
