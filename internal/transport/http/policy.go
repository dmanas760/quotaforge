package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/quotaforge/quotaforge/internal/domain"
	"github.com/quotaforge/quotaforge/internal/transport/http/middleware"
	"github.com/quotaforge/quotaforge/internal/transport/http/response"
	"github.com/quotaforge/quotaforge/internal/service"
)

// PolicyService manages policy lifecycle.
type PolicyService interface {
	Create(ctx context.Context, tenantID uuid.UUID, req service.CreatePolicyRequest) (*domain.Policy, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*domain.Policy, error)
	List(ctx context.Context, tenantID uuid.UUID) ([]*domain.Policy, error)
	Update(ctx context.Context, tenantID, id uuid.UUID, req service.UpdatePolicyRequest) (*domain.Policy, error)
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
}

// PolicyHandler handles policy CRUD endpoints.
type PolicyHandler struct {
	svc    PolicyService
	logger *slog.Logger
}

// NewPolicyHandler creates a new PolicyHandler.
func NewPolicyHandler(svc PolicyService, logger *slog.Logger) *PolicyHandler {
	return &PolicyHandler{svc: svc, logger: logger}
}

// Create handles POST /v1/policies.
func (h *PolicyHandler) Create(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	cred := middleware.GetCredential(r.Context())
	if cred == nil {
		response.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required", reqID)
		return
	}

	var req service.CreatePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body", reqID)
		return
	}
	if req.Name == "" {
		response.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required", reqID)
		return
	}
	if req.Algorithm != domain.AlgorithmTokenBucket && req.Algorithm != domain.AlgorithmSlidingWindow {
		response.WriteError(w, http.StatusBadRequest, "invalid_request", "algorithm must be token_bucket or sliding_window", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	policy, err := h.svc.Create(ctx, cred.TenantID, req)
	if err != nil {
		h.logger.ErrorContext(ctx, "policy create failed", "err", err, "request_id", reqID)
		response.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create policy", reqID)
		return
	}
	response.WriteJSON(w, http.StatusCreated, policy)
}

// Get handles GET /v1/policies/{id}.
func (h *PolicyHandler) Get(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	cred := middleware.GetCredential(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid policy ID", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	policy, err := h.svc.Get(ctx, cred.TenantID, id)
	if err != nil {
		if isNotFound(err) {
			response.WriteError(w, http.StatusNotFound, "not_found", "Policy not found", reqID)
			return
		}
		response.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get policy", reqID)
		return
	}
	response.WriteJSON(w, http.StatusOK, policy)
}

// List handles GET /v1/policies.
func (h *PolicyHandler) List(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	cred := middleware.GetCredential(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	policies, err := h.svc.List(ctx, cred.TenantID)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list policies", reqID)
		return
	}
	response.WriteJSON(w, http.StatusOK, map[string]any{"policies": policies})
}

// Update handles PATCH /v1/policies/{id}.
func (h *PolicyHandler) Update(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	cred := middleware.GetCredential(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid policy ID", reqID)
		return
	}

	var req service.UpdatePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	policy, err := h.svc.Update(ctx, cred.TenantID, id, req)
	if err != nil {
		if isConflict(err) {
			response.WriteError(w, http.StatusConflict, "version_conflict", "Policy was modified by another request; refresh and retry", reqID)
			return
		}
		if isNotFound(err) {
			response.WriteError(w, http.StatusNotFound, "not_found", "Policy not found", reqID)
			return
		}
		response.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update policy", reqID)
		return
	}
	response.WriteJSON(w, http.StatusOK, policy)
}

// Delete handles DELETE /v1/policies/{id}.
func (h *PolicyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	cred := middleware.GetCredential(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid policy ID", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.svc.Delete(ctx, cred.TenantID, id); err != nil {
		if isNotFound(err) {
			response.WriteError(w, http.StatusNotFound, "not_found", "Policy not found", reqID)
			return
		}
		response.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete policy", reqID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func isNotFound(err error) bool {
	_, ok := err.(domain.ErrNotFound)
	return ok
}

func isConflict(err error) bool {
	_, ok := err.(domain.ErrConflict)
	return ok
}
