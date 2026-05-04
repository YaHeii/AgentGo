package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/YaHeii/agentGo/internal/store"
)

func (s *Store) CreateSession(ctx context.Context, params store.CreateSessionParams) (store.Session, error) {
	row, err := s.q.CreateSession(ctx, CreateSessionParams{
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

func (s *Store) ListSessions(ctx context.Context) ([]store.Session, error) {
	rows, err := s.q.ListSessions(ctx)
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

func (s *Store) GetSession(ctx context.Context, id string) (store.Session, error) {
	row, err := s.q.GetSession(ctx, id)
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

func (s *Store) UpdateSession(ctx context.Context, params store.UpdateSessionParams) (store.Session, error) {
	row, err := s.q.UpdateSession(ctx, UpdateSessionParams{
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

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	rows, err := s.q.DeleteSession(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return store.ErrSessionNotFound
	}
	return nil
}
