package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type APIKeyRepo struct {
	db *DB
}

func NewAPIKeyRepo(db *DB) *APIKeyRepo { return &APIKeyRepo{db: db} }

func (r *APIKeyRepo) Upsert(ctx context.Context, k *APIKey) error {
	_, err := r.db.Pool.Exec(ctx, `INSERT INTO app.api_keys (id, instance_id, key_hash, key_prefix)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (instance_id) DO UPDATE SET key_hash = EXCLUDED.key_hash,
			key_prefix = EXCLUDED.key_prefix, created_at = now(), last_used_at = NULL`,
		k.ID, k.InstanceID, k.KeyHash, k.KeyPrefix)
	if err != nil {
		return fmt.Errorf("upsert api key: %w", err)
	}
	return nil
}

func (r *APIKeyRepo) GetByInstanceID(ctx context.Context, instanceID string) (*APIKey, error) {
	var k APIKey
	err := r.db.Pool.QueryRow(ctx, `SELECT id, instance_id, key_hash, key_prefix, created_at, last_used_at
		FROM app.api_keys WHERE instance_id = $1`, instanceID).
		Scan(&k.ID, &k.InstanceID, &k.KeyHash, &k.KeyPrefix, &k.CreatedAt, &k.LastUsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
	}
	return &k, nil
}

func (r *APIKeyRepo) FindInstanceByHash(ctx context.Context, keyHash string) (*Instance, error) {
	inst, err := scanInstance(r.db.Pool.QueryRow(ctx, `SELECT `+instanceColumnsAliased+`
		FROM app.instances i JOIN app.api_keys k ON k.instance_id = i.id
		WHERE k.key_hash = $1`, keyHash))
	if err != nil {
		return nil, fmt.Errorf("find instance by key: %w", err)
	}
	return inst, nil
}

func (r *APIKeyRepo) TouchLastUsed(ctx context.Context, keyHash string) error {
	_, err := r.db.Pool.Exec(ctx, `UPDATE app.api_keys SET last_used_at = now() WHERE key_hash = $1`, keyHash)
	return err
}

func (r *APIKeyRepo) DeleteByInstanceID(ctx context.Context, instanceID string) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM app.api_keys WHERE instance_id = $1`, instanceID)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	return nil
}
