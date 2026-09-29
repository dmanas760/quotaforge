package domain

import (
	"time"

	"github.com/google/uuid"
)

// Algorithm is the rate-limiting algorithm for a policy.
type Algorithm string

const (
	AlgorithmTokenBucket    Algorithm = "token_bucket"
	AlgorithmSlidingWindow  Algorithm = "sliding_window"
)

// CredentialStatus is the lifecycle state of a service API key.
type CredentialStatus string

const (
	CredentialStatusActive  CredentialStatus = "active"
	CredentialStatusRevoked CredentialStatus = "revoked"
)

// ----- Tenant ----------------------------------------------------------------

// Tenant is the top-level multi-tenant isolation unit.
type Tenant struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}

// ----- Credential ------------------------------------------------------------

// Credential is a hashed service API key used to authenticate API calls.
// Raw key is NEVER stored; only key_prefix (for support) and key_hash.
type Credential struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	KeyPrefix string           // first 8 chars of raw key, safe to log
	KeyHash   string           // Argon2id hash
	Status    CredentialStatus
	CreatedAt time.Time
	RevokedAt *time.Time
}

// ----- Policy ----------------------------------------------------------------

// Policy defines a rate-limiting rule.
type Policy struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	Name           string
	Algorithm      Algorithm
	Capacity       int64         // token bucket: max tokens; sliding window: max events
	RefillRate     float64       // token bucket: tokens added per second
	WindowSeconds  int64         // sliding window: interval in seconds
	BurstCapacity  int64         // token bucket: max burst (usually == Capacity)
	Enabled        bool
	Version        int           // optimistic concurrency version
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ----- Client Policy Assignment ---------------------------------------------

// ClientAssignment links a client ID to a policy, with optional JSON overrides.
type ClientAssignment struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	ClientID     string
	PolicyID     uuid.UUID
	OverrideJSON []byte    // nullable JSON; overrides specific policy fields
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ----- Audit event ----------------------------------------------------------

// AuditEvent records every configuration change.
type AuditEvent struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	ActorKeyID   uuid.UUID
	Action       string    // e.g. "policy.create", "key.revoke"
	ResourceType string    // e.g. "policy", "credential"
	ResourceID   uuid.UUID
	BeforeJSON   []byte
	AfterJSON    []byte
	CreatedAt    time.Time
}

// ----- Decision types -------------------------------------------------------

// DecisionRequest is the input to the rate limiter.
type DecisionRequest struct {
	TenantID  uuid.UUID
	ClientID  string
	PolicyID  *uuid.UUID // optional; resolved from assignment if absent
	Cost      int64      // defaults to 1
	RequestID string
}

// DecisionResult is the output of the rate limiter.
type DecisionResult struct {
	Allowed   bool
	Reason    string    // "ok" | "rate_limit_exceeded" | "policy_disabled" | ...
	Algorithm Algorithm
	Limit     int64
	Remaining int64
	ResetAt   time.Time
	RequestID string
}

// ----- Repository errors ----------------------------------------------------

// ErrNotFound is returned when a requested record does not exist.
type ErrNotFound struct{ Msg string }

func (e ErrNotFound) Error() string { return e.Msg }

// ErrConflict is returned on an optimistic version mismatch.
type ErrConflict struct{ Msg string }

func (e ErrConflict) Error() string { return e.Msg }

// ErrDependencyUnavailable is returned when Redis or PostgreSQL is unreachable.
type ErrDependencyUnavailable struct{ Msg string }

func (e ErrDependencyUnavailable) Error() string { return e.Msg }
