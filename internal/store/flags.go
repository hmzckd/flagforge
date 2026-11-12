package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Flag struct {
	Key         string
	Name        string
	Description string
	CreatedAt   time.Time
	Enabled     map[string]bool
}

type NewFlag struct {
	Key         string
	Name        string
	Description string
}

func (s *Store) CreateFlag(ctx context.Context, nf NewFlag) (Flag, error) {
	err := inTx(ctx, s.pool, func(tx pgx.Tx) error {
		var flagID int64
		err := tx.QueryRow(ctx,
			`INSERT INTO flags (key, name, description) VALUES ($1, $2, $3) RETURNING id`,
			nf.Key, nf.Name, nf.Description,
		).Scan(&flagID)
		if isUniqueViolation(err) {
			return ErrAlreadyExists
		}
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO flag_settings (flag_id, environment_id) SELECT $1, id FROM environments`,
			flagID,
		)
		return err
	})
	if err != nil {
		return Flag{}, err
	}
	return s.GetFlag(ctx, nf.Key)
}

const flagColumns = `
	SELECT f.key, f.name, f.description, f.created_at, e.key, fs.enabled
	FROM flags f
	JOIN flag_settings fs ON fs.flag_id = f.id
	JOIN environments e ON e.id = fs.environment_id`

func (s *Store) ListFlags(ctx context.Context) ([]Flag, error) {
	rows, err := s.pool.Query(ctx, flagColumns+` ORDER BY f.key, e.id`)
	if err != nil {
		return nil, err
	}
	return collectFlags(rows)
}

func (s *Store) GetFlag(ctx context.Context, key string) (Flag, error) {
	rows, err := s.pool.Query(ctx, flagColumns+` WHERE f.key = $1 ORDER BY e.id`, key)
	if err != nil {
		return Flag{}, err
	}
	flags, err := collectFlags(rows)
	if err != nil {
		return Flag{}, err
	}
	if len(flags) == 0 {
		return Flag{}, ErrFlagNotFound
	}
	return flags[0], nil
}

func collectFlags(rows pgx.Rows) ([]Flag, error) {
	defer rows.Close()
	flags := []Flag{}
	for rows.Next() {
		var f Flag
		var envKey string
		var enabled bool
		if err := rows.Scan(&f.Key, &f.Name, &f.Description, &f.CreatedAt, &envKey, &enabled); err != nil {
			return nil, err
		}
		if n := len(flags); n == 0 || flags[n-1].Key != f.Key {
			f.Enabled = map[string]bool{}
			flags = append(flags, f)
		}
		flags[len(flags)-1].Enabled[envKey] = enabled
	}
	return flags, rows.Err()
}

type FlagSetting struct {
	FlagKey     string
	Environment string
	Enabled     bool
	UpdatedAt   time.Time
}

type SettingChange struct {
	Enabled bool
}

func (s *Store) GetFlagSetting(ctx context.Context, flagKey, envKey string) (FlagSetting, error) {
	flagID, envID, err := resolve(ctx, s.pool, flagKey, envKey)
	if err != nil {
		return FlagSetting{}, err
	}
	fs := FlagSetting{FlagKey: flagKey, Environment: envKey}
	err = s.pool.QueryRow(ctx,
		`SELECT enabled, updated_at FROM flag_settings WHERE flag_id = $1 AND environment_id = $2`,
		flagID, envID,
	).Scan(&fs.Enabled, &fs.UpdatedAt)
	return fs, err
}

func (s *Store) UpdateFlagSetting(ctx context.Context, flagKey, envKey string, change SettingChange) (FlagSetting, error) {
	fs := FlagSetting{FlagKey: flagKey, Environment: envKey}
	err := inTx(ctx, s.pool, func(tx pgx.Tx) error {
		flagID, envID, err := resolve(ctx, tx, flagKey, envKey)
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`UPDATE flag_settings SET enabled = $3, updated_at = now()
			 WHERE flag_id = $1 AND environment_id = $2
			 RETURNING enabled, updated_at`,
			flagID, envID, change.Enabled,
		).Scan(&fs.Enabled, &fs.UpdatedAt)
	})
	if err != nil {
		return FlagSetting{}, err
	}
	return fs, nil
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func resolve(ctx context.Context, q querier, flagKey, envKey string) (flagID, envID int64, err error) {
	err = q.QueryRow(ctx, `SELECT id FROM environments WHERE key = $1`, envKey).Scan(&envID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, ErrEnvironmentNotFound
	}
	if err != nil {
		return 0, 0, err
	}
	err = q.QueryRow(ctx, `SELECT id FROM flags WHERE key = $1`, flagKey).Scan(&flagID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, ErrFlagNotFound
	}
	return flagID, envID, err
}
