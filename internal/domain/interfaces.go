package domain

import (
	"context"

	"github.com/google/uuid"
)

// PolicyRepository manages policy persistence.
type PolicyRepository interface {
	Create(ctx context.Context, p *Policy) error
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*Policy, error)
	List(ctx context.Context, tenantID uuid.UUID) ([]*Policy, error)
	Update(ctx context.Context, p *Policy) error // uses Version for optimistic lock
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
}

// CredentialRepository manages service API key persistence.
type CredentialRepository interface {
	Create(ctx context.Context, c *Credential) error
	GetByPrefix(ctx context.Context, keyPrefix string) (*Credential, error)
	List(ctx context.Context, tenantID uuid.UUID) ([]*Credential, error)
	Revoke(ctx context.Context, tenantID, id uuid.UUID) error
}

// AssignmentRepository manages client-to-policy assignments.
type AssignmentRepository interface {
	Upsert(ctx context.Context, a *ClientAssignment) error
	GetByClientID(ctx context.Context, tenantID uuid.UUID, clientID string) (*ClientAssignment, error)
	ListByPolicy(ctx context.Context, tenantID, policyID uuid.UUID) ([]*ClientAssignment, error)
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
}

// AuditRepository appends and queries audit events.
type AuditRepository interface {
	Append(ctx context.Context, e *AuditEvent) error
	// List returns events for a tenant, ordered by created_at desc, using cursor pagination.
	List(ctx context.Context, tenantID uuid.UUID, afterID *uuid.UUID, limit int) ([]*AuditEvent, error)
}

// TenantRepository manages tenant persistence.
type TenantRepository interface {
	Create(ctx context.Context, t *Tenant) error
	GetByID(ctx context.Context, id uuid.UUID) (*Tenant, error)
}

// Limiter is the rate-limiting port; implementations call Redis Lua scripts.
type Limiter interface {
	// Check atomically evaluates the rate limit and updates state.
	// A denied request MUST NOT reduce tokens or add a window event.
	Check(ctx context.Context, key LimiterKey, policy *Policy, cost int64) (*LimiterResult, error)
}

// LimiterKey uniquely identifies a rate-limit state bucket in Redis.
type LimiterKey struct {
	Tenant string
	Policy string
	Client string
}

// String renders the namespaced Redis key.
func (k LimiterKey) String() string {
	return "rl:" + k.Tenant + ":" + k.Policy + ":" + k.Client
}

// LimiterResult is the raw output from the Redis Lua script.
type LimiterResult struct {
	Allowed   bool
	Remaining int64
	ResetAtMs int64 // Unix milliseconds
}
