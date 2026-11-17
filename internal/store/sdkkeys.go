package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrSDKKeyNotFound = errors.New("sdk key not found")

const (
	sdkKeyPrefix     = "ffsdk_"
	sdkKeyShownChars = 10
)

type SDKKey struct {
	ID          int64
	Environment string
	Prefix      string
	CreatedAt   time.Time
	RevokedAt   *time.Time
}

func hashSDKKey(key string) []byte {
	sum := sha256.Sum256([]byte(key))
	return sum[:]
}

func (s *Store) CreateSDKKey(ctx context.Context, envKey string) (SDKKey, string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return SDKKey{}, "", err
	}
	plain := sdkKeyPrefix + base64.RawURLEncoding.EncodeToString(secret)

	k := SDKKey{Environment: envKey, Prefix: plain[:sdkKeyShownChars]}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO sdk_keys (environment_id, key_hash, prefix)
		 SELECT id, $2, $3 FROM environments WHERE key = $1
		 RETURNING id, created_at`,
		envKey, hashSDKKey(plain), k.Prefix,
	).Scan(&k.ID, &k.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SDKKey{}, "", ErrEnvironmentNotFound
	}
	if err != nil {
		return SDKKey{}, "", err
	}
	return k, plain, nil
}

func (s *Store) ListSDKKeys(ctx context.Context, envKey string) ([]SDKKey, error) {
	if _, err := s.environmentID(ctx, envKey); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT k.id, e.key, k.prefix, k.created_at, k.revoked_at
		 FROM sdk_keys k JOIN environments e ON e.id = k.environment_id
		 WHERE e.key = $1
		 ORDER BY k.id`,
		envKey,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (SDKKey, error) {
		var k SDKKey
		err := row.Scan(&k.ID, &k.Environment, &k.Prefix, &k.CreatedAt, &k.RevokedAt)
		return k, err
	})
}

func (s *Store) RevokeSDKKey(ctx context.Context, envKey string, id int64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE sdk_keys k SET revoked_at = coalesce(k.revoked_at, now())
		 FROM environments e
		 WHERE e.id = k.environment_id AND e.key = $1 AND k.id = $2`,
		envKey, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSDKKeyNotFound
	}
	return nil
}

func (s *Store) EnvironmentForSDKKey(ctx context.Context, key string) (string, error) {
	var envKey string
	err := s.pool.QueryRow(ctx,
		`SELECT e.key FROM sdk_keys k JOIN environments e ON e.id = k.environment_id
		 WHERE k.key_hash = $1 AND k.revoked_at IS NULL`,
		hashSDKKey(key),
	).Scan(&envKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrSDKKeyNotFound
	}
	return envKey, err
}

func (s *Store) environmentID(ctx context.Context, envKey string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM environments WHERE key = $1`, envKey).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrEnvironmentNotFound
	}
	return id, err
}
