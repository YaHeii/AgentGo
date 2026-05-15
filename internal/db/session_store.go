package db

import (
	"context"
	"database/sql"
	"errors"

	sessioncontract "github.com/YaHeii/agentGo/internal/session/contract"
)

func (s *Store) CreateSession(ctx context.Context, params sessioncontract.CreateSessionParams) (sessioncontract.Session, error) {
	return createSessionWithQuerier(ctx, s.q, params)
}

func (s *Store) ListSessions(ctx context.Context) ([]sessioncontract.Session, error) {
	return listSessionsWithQuerier(ctx, s.q)
}

func (s *Store) GetSession(ctx context.Context, id string) (sessioncontract.Session, error) {
	return getSessionWithQuerier(ctx, s.q, id)
}

func (s *Store) UpdateSession(ctx context.Context, params sessioncontract.UpdateSessionParams) (sessioncontract.Session, error) {
	return updateSessionWithQuerier(ctx, s.q, params)
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	return deleteSessionWithQuerier(ctx, s.q, id)
}

func (s *txStore) CreateSession(ctx context.Context, params sessioncontract.CreateSessionParams) (sessioncontract.Session, error) {
	return createSessionWithQuerier(ctx, s.q, params)
}

func (s *txStore) ListSessions(ctx context.Context) ([]sessioncontract.Session, error) {
	return listSessionsWithQuerier(ctx, s.q)
}

func (s *txStore) GetSession(ctx context.Context, id string) (sessioncontract.Session, error) {
	return getSessionWithQuerier(ctx, s.q, id)
}

func (s *txStore) UpdateSession(ctx context.Context, params sessioncontract.UpdateSessionParams) (sessioncontract.Session, error) {
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

func createSessionWithQuerier(ctx context.Context, q sessionQuerier, params sessioncontract.CreateSessionParams) (sessioncontract.Session, error) {
	row, err := q.CreateSession(ctx, CreateSessionParams{
		ID:               params.ID,
		ParentSessionID:  params.ParentSessionID,
		Title:            params.Title,
		MessageCount:     params.MessageCount,
		CompletionTokens: params.CompletionTokens,
		CostMicros:       params.CostMicros,
		SummaryMessageID: params.SummaryMessageID,
		TodosJson:        params.TodosJSON,
		CreatedAt:        params.CreatedAt.UTC().UnixMilli(),
		UpdatedAt:        params.UpdatedAt.UTC().UnixMilli(),
	})
	if err != nil {
		return sessioncontract.Session{}, err
	}

	return sessioncontract.Session{
		ID:               row.ID,
		ParentSessionID:  row.ParentSessionID,
		Title:            row.Title,
		MessageCount:     row.MessageCount,
		CompletionTokens: row.CompletionTokens,
		CostMicros:       row.CostMicros,
		SummaryMessageID: row.SummaryMessageID,
		TodosJSON:        row.TodosJson,
		CreatedAt:        unixMilliToTime(row.CreatedAt),
		UpdatedAt:        unixMilliToTime(row.UpdatedAt),
	}, nil
}

func listSessionsWithQuerier(ctx context.Context, q sessionQuerier) ([]sessioncontract.Session, error) {
	rows, err := q.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	sessions := make([]sessioncontract.Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, sessioncontract.Session{
			ID:               row.ID,
			ParentSessionID:  row.ParentSessionID,
			Title:            row.Title,
			MessageCount:     row.MessageCount,
			CompletionTokens: row.CompletionTokens,
			CostMicros:       row.CostMicros,
			SummaryMessageID: row.SummaryMessageID,
			TodosJSON:        row.TodosJson,
			CreatedAt:        unixMilliToTime(row.CreatedAt),
			UpdatedAt:        unixMilliToTime(row.UpdatedAt),
		})
	}

	return sessions, nil
}

func getSessionWithQuerier(ctx context.Context, q sessionQuerier, id string) (sessioncontract.Session, error) {
	row, err := q.GetSession(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return sessioncontract.Session{}, sessioncontract.ErrSessionNotFound
	}
	if err != nil {
		return sessioncontract.Session{}, err
	}

	return sessioncontract.Session{
		ID:               row.ID,
		ParentSessionID:  row.ParentSessionID,
		Title:            row.Title,
		MessageCount:     row.MessageCount,
		CompletionTokens: row.CompletionTokens,
		CostMicros:       row.CostMicros,
		SummaryMessageID: row.SummaryMessageID,
		TodosJSON:        row.TodosJson,
		CreatedAt:        unixMilliToTime(row.CreatedAt),
		UpdatedAt:        unixMilliToTime(row.UpdatedAt),
	}, nil
}

func updateSessionWithQuerier(ctx context.Context, q sessionQuerier, params sessioncontract.UpdateSessionParams) (sessioncontract.Session, error) {
	row, err := q.UpdateSession(ctx, UpdateSessionParams{
		ParentSessionID:  params.ParentSessionID,
		Title:            params.Title,
		MessageCount:     params.MessageCount,
		CompletionTokens: params.CompletionTokens,
		CostMicros:       params.CostMicros,
		SummaryMessageID: params.SummaryMessageID,
		TodosJson:        params.TodosJSON,
		UpdatedAt:        params.UpdatedAt.UTC().UnixMilli(),
		ID:               params.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return sessioncontract.Session{}, sessioncontract.ErrSessionNotFound
	}
	if err != nil {
		return sessioncontract.Session{}, err
	}

	return sessioncontract.Session{
		ID:               row.ID,
		ParentSessionID:  row.ParentSessionID,
		Title:            row.Title,
		MessageCount:     row.MessageCount,
		CompletionTokens: row.CompletionTokens,
		CostMicros:       row.CostMicros,
		SummaryMessageID: row.SummaryMessageID,
		TodosJSON:        row.TodosJson,
		CreatedAt:        unixMilliToTime(row.CreatedAt),
		UpdatedAt:        unixMilliToTime(row.UpdatedAt),
	}, nil
}

func deleteSessionWithQuerier(ctx context.Context, q sessionQuerier, id string) error {
	rows, err := q.DeleteSession(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return sessioncontract.ErrSessionNotFound
	}
	return nil
}
