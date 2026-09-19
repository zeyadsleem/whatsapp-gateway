package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type InstanceRepo struct {
	db *DB
}

func NewInstanceRepo(db *DB) *InstanceRepo { return &InstanceRepo{db: db} }

const instanceColumns = `id, shop_id, display_name, wa_jid, phone_number, status,
	webhook_url, webhook_secret_enc, webhook_enabled, created_at, updated_at, last_connected_at`

const instanceColumnsAliased = `i.id, i.shop_id, i.display_name, i.wa_jid, i.phone_number, i.status,
	i.webhook_url, i.webhook_secret_enc, i.webhook_enabled, i.created_at, i.updated_at, i.last_connected_at`

func scanInstance(row pgx.Row) (*Instance, error) {
	var i Instance
	err := row.Scan(&i.ID, &i.ShopID, &i.DisplayName, &i.WAJID, &i.PhoneNumber, &i.Status,
		&i.WebhookURL, &i.WebhookSecretEnc, &i.WebhookEnabled, &i.CreatedAt, &i.UpdatedAt, &i.LastConnectedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (r *InstanceRepo) Create(ctx context.Context, i *Instance) error {
	_, err := r.db.Pool.Exec(ctx, `INSERT INTO app.instances
		(id, shop_id, display_name, status, webhook_url, webhook_secret_enc, webhook_enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		i.ID, i.ShopID, i.DisplayName, i.Status, i.WebhookURL, i.WebhookSecretEnc, i.WebhookEnabled)
	if err != nil {
		if IsUniqueViolation(err) {
			return fmt.Errorf("%w: shop_id already exists", ErrConflict)
		}
		return fmt.Errorf("insert instance: %w", err)
	}
	return nil
}

func (r *InstanceRepo) GetByID(ctx context.Context, id string) (*Instance, error) {
	inst, err := scanInstance(r.db.Pool.QueryRow(ctx,
		`SELECT `+instanceColumns+` FROM app.instances WHERE id = $1`, id))
	if err != nil {
		return nil, fmt.Errorf("get instance %s: %w", id, err)
	}
	return inst, nil
}

func (r *InstanceRepo) GetByShopID(ctx context.Context, shopID string) (*Instance, error) {
	inst, err := scanInstance(r.db.Pool.QueryRow(ctx,
		`SELECT `+instanceColumns+` FROM app.instances WHERE shop_id = $1`, shopID))
	if err != nil {
		return nil, fmt.Errorf("get instance by shop %s: %w", shopID, err)
	}
	return inst, nil
}

func (r *InstanceRepo) List(ctx context.Context) ([]*Instance, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT `+instanceColumns+` FROM app.instances ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	defer rows.Close()
	return collectInstances(rows)
}

func (r *InstanceRepo) ListRestorable(ctx context.Context) ([]*Instance, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT `+instanceColumns+` FROM app.instances
		WHERE wa_jid IS NOT NULL AND status NOT IN ($1, $2)
		ORDER BY created_at ASC`, StatusLoggedOut, StatusError)
	if err != nil {
		return nil, fmt.Errorf("list restorable instances: %w", err)
	}
	defer rows.Close()
	return collectInstances(rows)
}

func collectInstances(rows pgx.Rows) ([]*Instance, error) {
	var out []*Instance
	for rows.Next() {
		var i Instance
		if err := rows.Scan(&i.ID, &i.ShopID, &i.DisplayName, &i.WAJID, &i.PhoneNumber, &i.Status,
			&i.WebhookURL, &i.WebhookSecretEnc, &i.WebhookEnabled, &i.CreatedAt, &i.UpdatedAt, &i.LastConnectedAt); err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		out = append(out, &i)
	}
	return out, rows.Err()
}

func (r *InstanceRepo) CountByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT status, count(*) FROM app.instances GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("count instances by status: %w", err)
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

func (r *InstanceRepo) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE app.instances SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("update instance status: %w", err)
	}
	return nil
}

func (r *InstanceRepo) SetIdentity(ctx context.Context, id, waJID, phone string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE app.instances SET wa_jid = $2, phone_number = $3, updated_at = now() WHERE id = $1`,
		id, waJID, phone)
	if err != nil {
		return fmt.Errorf("set instance identity: %w", err)
	}
	return nil
}

func (r *InstanceRepo) ClearIdentity(ctx context.Context, id string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE app.instances SET wa_jid = NULL, phone_number = NULL, updated_at = now() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("clear instance identity: %w", err)
	}
	return nil
}

func (r *InstanceRepo) MarkConnected(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE app.instances SET status = $2, last_connected_at = $3, updated_at = now() WHERE id = $1`,
		id, StatusConnected, at)
	if err != nil {
		return fmt.Errorf("mark instance connected: %w", err)
	}
	return nil
}

func (r *InstanceRepo) UpdateMeta(ctx context.Context, id, displayName, webhookURL string, webhookSecretEnc []byte, webhookEnabled bool) error {
	_, err := r.db.Pool.Exec(ctx, `UPDATE app.instances
		SET display_name = $2, webhook_url = NULLIF($3, ''), webhook_secret_enc = $4, webhook_enabled = $5, updated_at = now()
		WHERE id = $1`, id, displayName, webhookURL, webhookSecretEnc, webhookEnabled)
	if err != nil {
		return fmt.Errorf("update instance meta: %w", err)
	}
	return nil
}

func (r *InstanceRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `DELETE FROM app.instances WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete instance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *InstanceRepo) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.db.Pool.QueryRow(ctx, `SELECT count(*) FROM app.instances`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count instances: %w", err)
	}
	return n, nil
}
