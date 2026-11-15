package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/hmzckd/flagforge/eval"
)

type Flag struct {
	Key         string
	Name        string
	Description string
	CreatedAt   time.Time
	Enabled     map[string]bool
}

type NewFlag struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Store) CreateFlag(ctx context.Context, actor string, nf NewFlag) (Flag, error) {
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
		if err != nil {
			return err
		}
		return writeAudit(ctx, tx, auditEntry{flagID: flagID, actor: actor, action: ActionFlagCreated, after: nf})
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
	Rules       []eval.Rule
	Fallthrough eval.Serve
	Version     int64
	UpdatedAt   time.Time
}

func (s FlagSetting) EvalFlag() eval.Flag {
	return eval.Flag{Key: s.FlagKey, Enabled: s.Enabled, Rules: s.Rules, Fallthrough: s.Fallthrough}
}

type SettingChange struct {
	Enabled     bool        `json:"enabled"`
	Rules       []eval.Rule `json:"rules"`
	Fallthrough eval.Serve  `json:"fallthrough"`
}

func (s *Store) GetFlagSetting(ctx context.Context, flagKey, envKey string) (FlagSetting, error) {
	return getSetting(ctx, s.pool, flagKey, envKey)
}

func getSetting(ctx context.Context, q querier, flagKey, envKey string) (FlagSetting, error) {
	flagID, envID, err := resolve(ctx, q, flagKey, envKey)
	if err != nil {
		return FlagSetting{}, err
	}
	fs := FlagSetting{FlagKey: flagKey, Environment: envKey}
	err = q.QueryRow(ctx,
		`SELECT enabled, rules, fallthrough, version, updated_at
		 FROM flag_settings WHERE flag_id = $1 AND environment_id = $2`,
		flagID, envID,
	).Scan(&fs.Enabled, &fs.Rules, &fs.Fallthrough, &fs.Version, &fs.UpdatedAt)
	return fs, err
}

func (s *Store) UpdateFlagSetting(ctx context.Context, actor, flagKey, envKey string, expectedVersion int64, change SettingChange) (FlagSetting, error) {
	updated := FlagSetting{FlagKey: flagKey, Environment: envKey}
	err := inTx(ctx, s.pool, func(tx pgx.Tx) error {
		flagID, envID, err := resolve(ctx, tx, flagKey, envKey)
		if err != nil {
			return err
		}
		before, err := getSetting(ctx, tx, flagKey, envKey)
		if err != nil {
			return err
		}

		err = tx.QueryRow(ctx,
			`UPDATE flag_settings
			 SET enabled = $4, rules = $5, fallthrough = $6, version = version + 1, updated_at = now()
			 WHERE flag_id = $1 AND environment_id = $2 AND version = $3
			 RETURNING enabled, rules, fallthrough, version, updated_at`,
			flagID, envID, expectedVersion, change.Enabled, change.Rules, change.Fallthrough,
		).Scan(&updated.Enabled, &updated.Rules, &updated.Fallthrough, &updated.Version, &updated.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrVersionMismatch
		}
		if err != nil {
			return err
		}

		return writeAudit(ctx, tx, auditEntry{
			flagID:        flagID,
			environmentID: &envID,
			actor:         actor,
			action:        ActionSettingUpdated,
			before:        settingDocument(before),
			after:         settingDocument(updated),
		})
	})
	if err != nil {
		return FlagSetting{}, err
	}
	return updated, nil
}

func settingDocument(s FlagSetting) SettingChange {
	return SettingChange{Enabled: s.Enabled, Rules: s.Rules, Fallthrough: s.Fallthrough}
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
