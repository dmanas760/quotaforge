package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/quotaforge/quotaforge/internal/transport/http/middleware"
	"github.com/quotaforge/quotaforge/internal/transport/http/response"
)

// AssignmentService manages client-to-policy assignments.
type AssignmentService interface {
	AssignClientToPolicy(ctx context.Context, tenantID uuid.UUID, clientID string, policyID uuid.UUID) error
}

// AssignmentHandler handles POST /v1/assignments.
type AssignmentHandler struct {
	svc    AssignmentService
	logger *slog.Logger
}

// NewAssignmentHandler creates a new AssignmentHandler.
func NewAssignmentHandler(svc AssignmentService, logger *slog.Logger) *AssignmentHandler {
	return &AssignmentHandler{svc: svc, logger: logger}
}

// Assign handles POST /v1/assignments.
func (h *AssignmentHandler) Assign(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	cred := middleware.GetCredential(r.Context())
	if cred == nil {
		response.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required", reqID)
		return
	}

	var req struct {
		ClientID string    `json:"client_id"`
		PolicyID uuid.UUID `json:"policy_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body", reqID)
		return
	}
	if req.ClientID == "" {
		response.WriteError(w, http.StatusBadRequest, "invalid_request", "client_id is required", reqID)
		return
	}
	if req.PolicyID == uuid.Nil {
		response.WriteError(w, http.StatusBadRequest, "invalid_request", "policy_id is required", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.svc.AssignClientToPolicy(ctx, cred.TenantID, req.ClientID, req.PolicyID); err != nil {
		h.logger.ErrorContext(ctx, "assignment failed", "err", err, "request_id", reqID)
		response.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to assign client to policy", reqID)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}
