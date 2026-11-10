package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

type Flag struct {
	Key         string
	Name        string
	Description string
	CreatedAt   time.Time
}

type NewFlag struct {
	Key         string
	Name        string
	Description string
}

func (s *Store) CreateFlag(ctx context.Context, nf NewFlag) (Flag, error) {
	f := Flag{Key: nf.Key, Name: nf.Name, Description: nf.Description}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO flags (key, name, description) VALUES ($1, $2, $3) RETURNING created_at`,
		nf.Key, nf.Name, nf.Description,
	).Scan(&f.CreatedAt)
	if isUniqueViolation(err) {
		return Flag{}, ErrAlreadyExists
	}
	if err != nil {
		return Flag{}, err
	}
	return f, nil
}

func (s *Store) ListFlags(ctx context.Context) ([]Flag, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, name, description, created_at FROM flags ORDER BY key`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanFlag)
}

func (s *Store) GetFlag(ctx context.Context, key string) (Flag, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, name, description, created_at FROM flags WHERE key = $1`, key)
	if err != nil {
		return Flag{}, err
	}
	f, err := pgx.CollectExactlyOneRow(rows, scanFlag)
	if errors.Is(err, pgx.ErrNoRows) {
		return Flag{}, ErrNotFound
	}
	return f, err
}

func scanFlag(row pgx.CollectableRow) (Flag, error) {
	var f Flag
	err := row.Scan(&f.Key, &f.Name, &f.Description, &f.CreatedAt)
	return f, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
