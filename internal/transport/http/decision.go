package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/quotaforge/quotaforge/internal/domain"
	"github.com/quotaforge/quotaforge/internal/transport/http/middleware"
	"github.com/quotaforge/quotaforge/internal/transport/http/response"
)

// DecisionService processes rate-limit decisions.
type DecisionService interface {
	Decide(ctx context.Context, req *domain.DecisionRequest) (*domain.DecisionResult, error)
}

// DecisionHandler handles POST /v1/decisions.
type DecisionHandler struct {
	svc    DecisionService
	logger *slog.Logger
}

// NewDecisionHandler creates a new DecisionHandler.
func NewDecisionHandler(svc DecisionService, logger *slog.Logger) *DecisionHandler {
	return &DecisionHandler{svc: svc, logger: logger}
}

type decisionRequest struct {
	ClientID string  `json:"client_id" validate:"required"`
	PolicyID *string `json:"policy_id,omitempty"`
	Cost     int64   `json:"cost"`
}

// Handle processes a rate-limit decision request.
func (h *DecisionHandler) Handle(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	cred := middleware.GetCredential(r.Context())
	if cred == nil {
		response.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required", reqID)
		return
	}

	var body decisionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body", reqID)
		return
	}
	if body.ClientID == "" {
		response.WriteError(w, http.StatusBadRequest, "invalid_request", "client_id is required", reqID)
		return
	}
	if body.Cost == 0 {
		body.Cost = 1
	}
	if body.Cost < 0 {
		response.WriteError(w, http.StatusBadRequest, "invalid_request", "cost must be positive", reqID)
		return
	}

	req := &domain.DecisionRequest{
		TenantID:  cred.TenantID,
		ClientID:  body.ClientID,
		Cost:      body.Cost,
		RequestID: reqID,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := h.svc.Decide(ctx, req)
	if err != nil {
		h.logger.ErrorContext(ctx, "decision failed", "err", err, "request_id", reqID)
		response.WriteError(w, http.StatusServiceUnavailable, "dependency_unavailable", "Service temporarily unavailable", reqID)
		return
	}

	// Set standard IETF rate-limit headers
	w.Header().Set("RateLimit-Limit", itoa(result.Limit))
	w.Header().Set("RateLimit-Remaining", itoa(result.Remaining))
	w.Header().Set("RateLimit-Reset", result.ResetAt.UTC().Format(time.RFC3339))
	if !result.Allowed {
		retryAfter := int64(time.Until(result.ResetAt).Seconds())
		if retryAfter < 0 {
			retryAfter = 0
		}
		w.Header().Set("Retry-After", itoa(retryAfter))
	}

	response.WriteJSON(w, http.StatusOK, map[string]any{
		"allowed":    result.Allowed,
		"reason":     result.Reason,
		"algorithm":  result.Algorithm,
		"limit":      result.Limit,
		"remaining":  result.Remaining,
		"reset_at":   result.ResetAt.UTC().Format(time.RFC3339),
		"request_id": result.RequestID,
	})
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
