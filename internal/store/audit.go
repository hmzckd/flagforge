package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	ActionFlagCreated    = "flag.created"
	ActionSettingUpdated = "flag.setting_updated"
)

const auditPageSize = 100

type AuditEntry struct {
	ID          int64
	Environment string
	Actor       string
	Action      string
	Before      json.RawMessage
	After       json.RawMessage
	CreatedAt   time.Time
}

type auditEntry struct {
	flagID        int64
	environmentID *int64
	actor         string
	action        string
	before        any
	after         any
}

func writeAudit(ctx context.Context, tx pgx.Tx, e auditEntry) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO audit_entries (flag_id, environment_id, actor, action, before, after)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		e.flagID, e.environmentID, e.actor, e.action, e.before, e.after,
	)
	return err
}

func (s *Store) ListAudit(ctx context.Context, flagKey string) ([]AuditEntry, error) {
	var flagID int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM flags WHERE key = $1`, flagKey).Scan(&flagID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFlagNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT a.id, coalesce(e.key, ''), a.actor, a.action, a.before, a.after, a.created_at
		 FROM audit_entries a
		 LEFT JOIN environments e ON e.id = a.environment_id
		 WHERE a.flag_id = $1
		 ORDER BY a.id DESC
		 LIMIT $2`,
		flagID, auditPageSize,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AuditEntry, error) {
		var a AuditEntry
		err := row.Scan(&a.ID, &a.Environment, &a.Actor, &a.Action, &a.Before, &a.After, &a.CreatedAt)
		return a, err
	})
}
