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

// AssignmentRepo implements domain.AssignmentRepository.
type AssignmentRepo struct {
	pool *pgxpool.Pool
}

// NewAssignmentRepo creates a new AssignmentRepo.
func NewAssignmentRepo(pool *pgxpool.Pool) *AssignmentRepo {
	return &AssignmentRepo{pool: pool}
}

// Upsert creates or updates a client-policy assignment.
func (r *AssignmentRepo) Upsert(ctx context.Context, a *domain.ClientAssignment) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now

	_, err := r.pool.Exec(ctx, `
		INSERT INTO client_policy_assignments (id, tenant_id, client_id, policy_id, override_json, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (tenant_id, client_id) DO UPDATE
		  SET policy_id=EXCLUDED.policy_id,
		      override_json=EXCLUDED.override_json,
		      updated_at=EXCLUDED.updated_at`,
		a.ID, a.TenantID, a.ClientID, a.PolicyID, a.OverrideJSON, a.CreatedAt, a.UpdatedAt,
	)
	return err
}

// GetByClientID retrieves the policy assignment for a client.
func (r *AssignmentRepo) GetByClientID(ctx context.Context, tenantID uuid.UUID, clientID string) (*domain.ClientAssignment, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, client_id, policy_id, override_json, created_at, updated_at
		FROM client_policy_assignments WHERE tenant_id=$1 AND client_id=$2`, tenantID, clientID)

	var a domain.ClientAssignment
	err := row.Scan(&a.ID, &a.TenantID, &a.ClientID, &a.PolicyID, &a.OverrideJSON, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound{Msg: "client assignment not found"}
	}
	return &a, err
}

// ListByPolicy returns all assignments for a policy.
func (r *AssignmentRepo) ListByPolicy(ctx context.Context, tenantID, policyID uuid.UUID) ([]*domain.ClientAssignment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, client_id, policy_id, override_json, created_at, updated_at
		FROM client_policy_assignments WHERE tenant_id=$1 AND policy_id=$2`, tenantID, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []*domain.ClientAssignment
	for rows.Next() {
		var a domain.ClientAssignment
		if err := rows.Scan(&a.ID, &a.TenantID, &a.ClientID, &a.PolicyID, &a.OverrideJSON, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		assignments = append(assignments, &a)
	}
	return assignments, rows.Err()
}

// Delete removes an assignment.
func (r *AssignmentRepo) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM client_policy_assignments WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound{Msg: "assignment not found"}
	}
	return nil
}
