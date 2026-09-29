package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/quotaforge/quotaforge/internal/domain"
)

// CredentialRepo implements domain.CredentialRepository with pgx.
type CredentialRepo struct {
	pool *pgxpool.Pool
}

// NewCredentialRepo creates a new CredentialRepo.
func NewCredentialRepo(pool *pgxpool.Pool) *CredentialRepo {
	return &CredentialRepo{pool: pool}
}

// Create inserts a new credential.
func (r *CredentialRepo) Create(ctx context.Context, c *domain.Credential) error {
	c.ID = uuid.New()
	c.Status = domain.CredentialStatusActive
	c.CreatedAt = time.Now().UTC()

	_, err := r.pool.Exec(ctx, `
		INSERT INTO service_api_keys (id, tenant_id, key_prefix, key_hash, status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		c.ID, c.TenantID, c.KeyPrefix, c.KeyHash, c.Status, c.CreatedAt,
	)
	return err
}

// GetByPrefix retrieves an active credential by key prefix.
func (r *CredentialRepo) GetByPrefix(ctx context.Context, keyPrefix string) (*domain.Credential, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, key_prefix, key_hash, status, created_at, revoked_at
		FROM service_api_keys WHERE key_prefix=$1`, keyPrefix)

	return scanCredential(row)
}

// List returns all credentials for a tenant (hashes excluded for safety).
func (r *CredentialRepo) List(ctx context.Context, tenantID uuid.UUID) ([]*domain.Credential, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, key_prefix, key_hash, status, created_at, revoked_at
		FROM service_api_keys WHERE tenant_id=$1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []*domain.Credential
	for rows.Next() {
		c, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// Revoke marks a credential as revoked.
func (r *CredentialRepo) Revoke(ctx context.Context, tenantID, id uuid.UUID) error {
	now := time.Now().UTC()
	tag, err := r.pool.Exec(ctx, `
		UPDATE service_api_keys SET status='revoked', revoked_at=$1
		WHERE id=$2 AND tenant_id=$3 AND status='active'`,
		now, id, tenantID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound{Msg: "credential not found or already revoked"}
	}
	return nil
}

func scanCredential(row rowScanner) (*domain.Credential, error) {
	var c domain.Credential
	err := row.Scan(&c.ID, &c.TenantID, &c.KeyPrefix, &c.KeyHash, &c.Status, &c.CreatedAt, &c.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound{Msg: "credential not found"}
	}
	return &c, err
}
