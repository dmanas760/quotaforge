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

// AuditRepo implements domain.AuditRepository.
type AuditRepo struct {
	pool *pgxpool.Pool
}

// NewAuditRepo creates a new AuditRepo.
func NewAuditRepo(pool *pgxpool.Pool) *AuditRepo {
	return &AuditRepo{pool: pool}
}

// Append inserts an audit event.
func (r *AuditRepo) Append(ctx context.Context, e *domain.AuditEvent) error {
	e.ID = uuid.New()
	e.CreatedAt = time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_events
			(id, tenant_id, actor_key_id, action, resource_type, resource_id, before_json, after_json, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		e.ID, e.TenantID, e.ActorKeyID, e.Action, e.ResourceType,
		e.ResourceID, e.BeforeJSON, e.AfterJSON, e.CreatedAt,
	)
	return err
}

// List returns audit events for a tenant using keyset pagination.
func (r *AuditRepo) List(ctx context.Context, tenantID uuid.UUID, afterID *uuid.UUID, limit int) ([]*domain.AuditEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var rows pgx.Rows
	var err error

	if afterID == nil {
		rows, err = r.pool.Query(ctx, `
			SELECT id, tenant_id, actor_key_id, action, resource_type, resource_id,
			       before_json, after_json, created_at
			FROM audit_events WHERE tenant_id=$1
			ORDER BY created_at DESC LIMIT $2`, tenantID, limit)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT id, tenant_id, actor_key_id, action, resource_type, resource_id,
			       before_json, after_json, created_at
			FROM audit_events WHERE tenant_id=$1 AND created_at < (
				SELECT created_at FROM audit_events WHERE id=$2
			)
			ORDER BY created_at DESC LIMIT $3`, tenantID, afterID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*domain.AuditEvent
	for rows.Next() {
		var e domain.AuditEvent
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ActorKeyID, &e.Action, &e.ResourceType,
			&e.ResourceID, &e.BeforeJSON, &e.AfterJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}

// unused but satisfies the rowScanner interface across files
var _ = errors.New
