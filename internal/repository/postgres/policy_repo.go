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

// PolicyRepo implements domain.PolicyRepository with pgx.
type PolicyRepo struct {
	pool *pgxpool.Pool
}

// NewPolicyRepo creates a new PolicyRepo.
func NewPolicyRepo(pool *pgxpool.Pool) *PolicyRepo {
	return &PolicyRepo{pool: pool}
}

// Create inserts a new policy.
func (r *PolicyRepo) Create(ctx context.Context, p *domain.Policy) error {
	p.ID = uuid.New()
	p.Version = 1
	p.CreatedAt = time.Now().UTC()
	p.UpdatedAt = p.CreatedAt
	p.Enabled = true

	_, err := r.pool.Exec(ctx, `
		INSERT INTO policies
			(id, tenant_id, name, algorithm, capacity, refill_rate, window_seconds, burst_capacity, enabled, version, created_at, updated_at)
		VALUES
			($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		p.ID, p.TenantID, p.Name, p.Algorithm, p.Capacity, p.RefillRate,
		p.WindowSeconds, p.BurstCapacity, p.Enabled, p.Version, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

// GetByID retrieves a policy by tenant+ID.
func (r *PolicyRepo) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*domain.Policy, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, algorithm, capacity, refill_rate, window_seconds,
		       burst_capacity, enabled, version, created_at, updated_at
		FROM policies WHERE id=$1 AND tenant_id=$2`, id, tenantID)

	return scanPolicy(row)
}

// List returns all policies for a tenant.
func (r *PolicyRepo) List(ctx context.Context, tenantID uuid.UUID) ([]*domain.Policy, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, algorithm, capacity, refill_rate, window_seconds,
		       burst_capacity, enabled, version, created_at, updated_at
		FROM policies WHERE tenant_id=$1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []*domain.Policy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

// Update performs an optimistic-lock update.
func (r *PolicyRepo) Update(ctx context.Context, p *domain.Policy) error {
	p.UpdatedAt = time.Now().UTC()
	tag, err := r.pool.Exec(ctx, `
		UPDATE policies SET
			name=$1, capacity=$2, refill_rate=$3, window_seconds=$4,
			burst_capacity=$5, enabled=$6, version=version+1, updated_at=$7
		WHERE id=$8 AND tenant_id=$9 AND version=$10`,
		p.Name, p.Capacity, p.RefillRate, p.WindowSeconds,
		p.BurstCapacity, p.Enabled, p.UpdatedAt,
		p.ID, p.TenantID, p.Version,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict{Msg: "policy version conflict or not found"}
	}
	return nil
}

// Delete removes a policy.
func (r *PolicyRepo) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM policies WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound{Msg: "policy not found"}
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPolicy(row rowScanner) (*domain.Policy, error) {
	var p domain.Policy
	err := row.Scan(
		&p.ID, &p.TenantID, &p.Name, &p.Algorithm, &p.Capacity, &p.RefillRate,
		&p.WindowSeconds, &p.BurstCapacity, &p.Enabled, &p.Version, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound{Msg: "policy not found"}
	}
	return &p, err
}
