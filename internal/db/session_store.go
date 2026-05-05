package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/YaHeii/agentGo/internal/store"
)

func (s *Store) CreateSession(ctx context.Context, params store.CreateSessionParams) (store.Session, error) {
	return createSessionWithQuerier(ctx, s.q, params)
}

func (s *Store) ListSessions(ctx context.Context) ([]store.Session, error) {
	return listSessionsWithQuerier(ctx, s.q)
}

func (s *Store) GetSession(ctx context.Context, id string) (store.Session, error) {
	return getSessionWithQuerier(ctx, s.q, id)
}

func (s *Store) UpdateSession(ctx context.Context, params store.UpdateSessionParams) (store.Session, error) {
	return updateSessionWithQuerier(ctx, s.q, params)
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	return deleteSessionWithQuerier(ctx, s.q, id)
}

func (s *txStore) CreateSession(ctx context.Context, params store.CreateSessionParams) (store.Session, error) {
	return createSessionWithQuerier(ctx, s.q, params)
}

func (s *txStore) ListSessions(ctx context.Context) ([]store.Session, error) {
	return listSessionsWithQuerier(ctx, s.q)
}

func (s *txStore) GetSession(ctx context.Context, id string) (store.Session, error) {
	return getSessionWithQuerier(ctx, s.q, id)
}

func (s *txStore) UpdateSession(ctx context.Context, params store.UpdateSessionParams) (store.Session, error) {
	return updateSessionWithQuerier(ctx, s.q, params)
}

func (s *txStore) DeleteSession(ctx context.Context, id string) error {
	return deleteSessionWithQuerier(ctx, s.q, id)
}

type sessionQuerier interface {
	CreateSession(ctx context.Context, arg CreateSessionParams) (Session, error)
	ListSessions(ctx context.Context) ([]Session, error)
	GetSession(ctx context.Context, id string) (Session, error)
	UpdateSession(ctx context.Context, arg UpdateSessionParams) (Session, error)
	DeleteSession(ctx context.Context, id string) (int64, error)
}

func createSessionWithQuerier(ctx context.Context, q sessionQuerier, params store.CreateSessionParams) (store.Session, error) {
	row, err := q.CreateSession(ctx, CreateSessionParams{
		ID:           params.ID,
		Title:        params.Title,
		CreatedAt:    params.CreatedAt.UTC().UnixMilli(),
		UpdatedAt:    params.UpdatedAt.UTC().UnixMilli(),
		LastActiveAt: params.LastActiveAt.UTC().UnixMilli(),
	})
	if err != nil {
		return store.Session{}, err
	}

	return store.Session{
		ID:           row.ID,
		Title:        row.Title,
		CreatedAt:    unixMilliToTime(row.CreatedAt),
		UpdatedAt:    unixMilliToTime(row.UpdatedAt),
		LastActiveAt: unixMilliToTime(row.LastActiveAt),
	}, nil
}

func listSessionsWithQuerier(ctx context.Context, q sessionQuerier) ([]store.Session, error) {
	rows, err := q.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	sessions := make([]store.Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, store.Session{
			ID:           row.ID,
			Title:        row.Title,
			CreatedAt:    unixMilliToTime(row.CreatedAt),
			UpdatedAt:    unixMilliToTime(row.UpdatedAt),
			LastActiveAt: unixMilliToTime(row.LastActiveAt),
		})
	}

	return sessions, nil
}

func getSessionWithQuerier(ctx context.Context, q sessionQuerier, id string) (store.Session, error) {
	row, err := q.GetSession(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Session{}, store.ErrSessionNotFound
	}
	if err != nil {
		return store.Session{}, err
	}

	return store.Session{
		ID:           row.ID,
		Title:        row.Title,
		CreatedAt:    unixMilliToTime(row.CreatedAt),
		UpdatedAt:    unixMilliToTime(row.UpdatedAt),
		LastActiveAt: unixMilliToTime(row.LastActiveAt),
	}, nil
}

func updateSessionWithQuerier(ctx context.Context, q sessionQuerier, params store.UpdateSessionParams) (store.Session, error) {
	row, err := q.UpdateSession(ctx, UpdateSessionParams{
		Title:        params.Title,
		UpdatedAt:    params.UpdatedAt.UTC().UnixMilli(),
		LastActiveAt: params.LastActiveAt.UTC().UnixMilli(),
		ID:           params.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return store.Session{}, store.ErrSessionNotFound
	}
	if err != nil {
		return store.Session{}, err
	}

	return store.Session{
		ID:           row.ID,
		Title:        row.Title,
		CreatedAt:    unixMilliToTime(row.CreatedAt),
		UpdatedAt:    unixMilliToTime(row.UpdatedAt),
		LastActiveAt: unixMilliToTime(row.LastActiveAt),
	}, nil
}

func deleteSessionWithQuerier(ctx context.Context, q sessionQuerier, id string) error {
	rows, err := q.DeleteSession(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return store.ErrSessionNotFound
	}
	return nil
}
