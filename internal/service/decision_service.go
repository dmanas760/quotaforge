package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/quotaforge/quotaforge/internal/domain"
	"github.com/quotaforge/quotaforge/internal/observability"
)

// DecisionService selects the right limiter and records metrics.
type DecisionService struct {
	policies       *PolicyService
	assignments    domain.AssignmentRepository
	tokenBucket    domain.Limiter
	slidingWindow  domain.Limiter
	metrics        *observability.Metrics
}

// NewDecisionService creates a DecisionService.
func NewDecisionService(
	policies *PolicyService,
	assignments domain.AssignmentRepository,
	tokenBucket domain.Limiter,
	slidingWindow domain.Limiter,
	metrics *observability.Metrics,
) *DecisionService {
	return &DecisionService{
		policies:      policies,
		assignments:   assignments,
		tokenBucket:   tokenBucket,
		slidingWindow: slidingWindow,
		metrics:       metrics,
	}
}

// Decide resolves the policy for the client and runs the rate limiter.
func (s *DecisionService) Decide(ctx context.Context, req *domain.DecisionRequest) (*domain.DecisionResult, error) {
	start := time.Now()

	// Resolve policy via assignment
	assignment, err := s.assignments.GetByClientID(ctx, req.TenantID, req.ClientID)
	if err != nil {
		return nil, fmt.Errorf("resolving assignment: %w", domain.ErrDependencyUnavailable{Msg: err.Error()})
	}

	policyID := assignment.PolicyID
	if req.PolicyID != nil {
		policyID = *req.PolicyID
	}

	policy, err := s.policies.GetCached(ctx, req.TenantID, policyID)
	if err != nil {
		return nil, fmt.Errorf("resolving policy: %w", err)
	}

	if !policy.Enabled {
		return &domain.DecisionResult{
			Allowed:   false,
			Reason:    "policy_disabled",
			Algorithm: policy.Algorithm,
			Limit:     policy.Capacity,
			Remaining: 0,
			ResetAt:   time.Now().UTC(),
			RequestID: req.RequestID,
		}, nil
	}

	// Select limiter
	limiter := s.tokenBucket
	if policy.Algorithm == domain.AlgorithmSlidingWindow {
		limiter = s.slidingWindow
	}

	key := domain.LimiterKey{
		Tenant: req.TenantID.String(),
		Policy: policyID.String(),
		Client: req.ClientID,
	}

	limResult, err := limiter.Check(ctx, key, policy, req.Cost)
	if err != nil {
		s.metrics.DependencyErrors.WithLabelValues("redis").Inc()
		return nil, fmt.Errorf("limiter check: %w", domain.ErrDependencyUnavailable{Msg: err.Error()})
	}

	algoStr := string(policy.Algorithm)
	result := "allowed"
	if !limResult.Allowed {
		result = "denied"
	}

	s.metrics.DecisionsTotal.WithLabelValues(algoStr, result).Inc()
	s.metrics.DecisionDuration.WithLabelValues(algoStr).Observe(time.Since(start).Seconds())

	reason := "ok"
	if !limResult.Allowed {
		reason = "rate_limit_exceeded"
	}

	return &domain.DecisionResult{
		Allowed:   limResult.Allowed,
		Reason:    reason,
		Algorithm: policy.Algorithm,
		Limit:     policy.Capacity,
		Remaining: limResult.Remaining,
		ResetAt:   time.UnixMilli(limResult.ResetAtMs).UTC(),
		RequestID: req.RequestID,
	}, nil
}

// AssignClientToPolicy creates or updates a client-policy assignment.
func (s *DecisionService) AssignClientToPolicy(ctx context.Context, tenantID uuid.UUID, clientID string, policyID uuid.UUID) error {
	a := &domain.ClientAssignment{
		TenantID: tenantID,
		ClientID: clientID,
		PolicyID: policyID,
	}
	return s.assignments.Upsert(ctx, a)
}
