package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/quotaforge/quotaforge/internal/domain"
	"github.com/quotaforge/quotaforge/internal/observability"
)

// PolicyService manages policy lifecycle with an in-process cache.
type PolicyService struct {
	repo    domain.PolicyRepository
	audit   domain.AuditRepository
	metrics *observability.Metrics

	mu    sync.RWMutex
	cache map[string]*cacheEntry
}

type cacheEntry struct {
	policy    *domain.Policy
	expiresAt time.Time
}

const cacheTTL = 30 * time.Second

// NewPolicyService creates a PolicyService.
func NewPolicyService(repo domain.PolicyRepository, audit domain.AuditRepository, metrics *observability.Metrics) *PolicyService {
	return &PolicyService{
		repo:    repo,
		audit:   audit,
		metrics: metrics,
		cache:   make(map[string]*cacheEntry),
	}
}

// cacheKey builds a cache key from tenant+policy IDs.
func cacheKey(tenantID, policyID uuid.UUID) string {
	return tenantID.String() + ":" + policyID.String()
}

// GetCached returns a policy from cache or Postgres.
func (s *PolicyService) GetCached(ctx context.Context, tenantID, policyID uuid.UUID) (*domain.Policy, error) {
	ck := cacheKey(tenantID, policyID)

	s.mu.RLock()
	entry, ok := s.cache[ck]
	s.mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		s.metrics.PolicyCacheHits.Inc()
		return entry.policy, nil
	}

	s.metrics.PolicyCacheMisses.Inc()
	policy, err := s.repo.GetByID(ctx, tenantID, policyID)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cache[ck] = &cacheEntry{policy: policy, expiresAt: time.Now().Add(cacheTTL)}
	s.mu.Unlock()

	return policy, nil
}

// invalidate removes a policy from cache.
func (s *PolicyService) invalidate(tenantID, policyID uuid.UUID) {
	s.mu.Lock()
	delete(s.cache, cacheKey(tenantID, policyID))
	s.mu.Unlock()
}

// Create creates a new policy.
func (s *PolicyService) Create(ctx context.Context, tenantID uuid.UUID, req CreatePolicyRequest) (*domain.Policy, error) {
	p := &domain.Policy{
		TenantID:      tenantID,
		Name:          req.Name,
		Algorithm:     req.Algorithm,
		Capacity:      req.Capacity,
		RefillRate:    req.RefillRate,
		WindowSeconds: req.WindowSeconds,
		BurstCapacity: req.BurstCapacity,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create policy: %w", err)
	}
	return p, nil
}

// Get retrieves a policy by ID.
func (s *PolicyService) Get(ctx context.Context, tenantID, id uuid.UUID) (*domain.Policy, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

// List returns all policies for a tenant.
func (s *PolicyService) List(ctx context.Context, tenantID uuid.UUID) ([]*domain.Policy, error) {
	return s.repo.List(ctx, tenantID)
}

// Update performs an optimistic-lock update and invalidates cache.
func (s *PolicyService) Update(ctx context.Context, tenantID, id uuid.UUID, req UpdatePolicyRequest) (*domain.Policy, error) {
	p, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Capacity != nil {
		p.Capacity = *req.Capacity
	}
	if req.RefillRate != nil {
		p.RefillRate = *req.RefillRate
	}
	if req.WindowSeconds != nil {
		p.WindowSeconds = *req.WindowSeconds
	}
	if req.BurstCapacity != nil {
		p.BurstCapacity = *req.BurstCapacity
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	p.Version = req.Version

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	s.invalidate(tenantID, id)
	return p, nil
}

// Delete removes a policy and invalidates cache.
func (s *PolicyService) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, tenantID, id); err != nil {
		return err
	}
	s.invalidate(tenantID, id)
	return nil
}

// CreatePolicyRequest holds creation parameters.
type CreatePolicyRequest struct {
	Name          string           `json:"name"`
	Algorithm     domain.Algorithm `json:"algorithm"`
	Capacity      int64            `json:"capacity"`
	RefillRate    float64          `json:"refill_rate"`
	WindowSeconds int64            `json:"window_seconds"`
	BurstCapacity int64            `json:"burst_capacity"`
}

// UpdatePolicyRequest holds update parameters.
type UpdatePolicyRequest struct {
	Name          *string  `json:"name,omitempty"`
	Capacity      *int64   `json:"capacity,omitempty"`
	RefillRate    *float64 `json:"refill_rate,omitempty"`
	WindowSeconds *int64   `json:"window_seconds,omitempty"`
	BurstCapacity *int64   `json:"burst_capacity,omitempty"`
	Enabled       *bool    `json:"enabled,omitempty"`
	Version       int      `json:"version"`
}
