package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/hmzckd/flagforge/eval"
)

type Snapshot struct {
	Environment string               `json:"environment"`
	Version     int64                `json:"version"`
	Flags       map[string]eval.Flag `json:"flags"`
}

func (s *Store) EnvironmentVersion(ctx context.Context, envKey string) (int64, error) {
	var version int64
	err := s.pool.QueryRow(ctx, `SELECT version FROM environments WHERE key = $1`, envKey).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrEnvironmentNotFound
	}
	return version, err
}

func (s *Store) Snapshot(ctx context.Context, envKey string) (Snapshot, error) {
	snap := Snapshot{Environment: envKey, Flags: map[string]eval.Flag{}}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Snapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var envID int64
	err = tx.QueryRow(ctx, `SELECT id, version FROM environments WHERE key = $1`, envKey).Scan(&envID, &snap.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{}, ErrEnvironmentNotFound
	}
	if err != nil {
		return Snapshot{}, err
	}

	rows, err := tx.Query(ctx,
		`SELECT f.key, fs.enabled, fs.rules, fs.fallthrough
		 FROM flag_settings fs JOIN flags f ON f.id = fs.flag_id
		 WHERE fs.environment_id = $1`,
		envID,
	)
	if err != nil {
		return Snapshot{}, err
	}
	flags, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (eval.Flag, error) {
		var f eval.Flag
		err := row.Scan(&f.Key, &f.Enabled, &f.Rules, &f.Fallthrough)
		return f, err
	})
	if err != nil {
		return Snapshot{}, err
	}
	for _, f := range flags {
		snap.Flags[f.Key] = f
	}
	return snap, tx.Commit(ctx)
}

func bumpEnvironmentVersions(ctx context.Context, tx pgx.Tx, envID *int64) error {
	if envID == nil {
		_, err := tx.Exec(ctx, `UPDATE environments SET version = version + 1`)
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE environments SET version = version + 1 WHERE id = $1`, *envID)
	return err
}
