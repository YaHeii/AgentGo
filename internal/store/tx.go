package store

import "context"

type TxStore interface {
	SessionStore
	MessageStore
	DraftStore
}

type Transactor interface {
	WithinTx(ctx context.Context, fn func(tx TxStore) error) error
}
