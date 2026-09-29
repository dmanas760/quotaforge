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
)

// KeyService manages service API key lifecycle.
type KeyService interface {
	Create(ctx context.Context, tenantID uuid.UUID, name string) (*domain.Credential, string, error) // returns cred + raw key
	List(ctx context.Context, tenantID uuid.UUID) ([]*domain.Credential, error)
	Rotate(ctx context.Context, tenantID, id uuid.UUID) (*domain.Credential, string, error) // returns cred + new raw key
	Revoke(ctx context.Context, tenantID, id uuid.UUID) error
}

// KeyHandler handles service API key management endpoints.
type KeyHandler struct {
	svc    KeyService
	logger *slog.Logger
}

// NewKeyHandler creates a new KeyHandler.
func NewKeyHandler(svc KeyService, logger *slog.Logger) *KeyHandler {
	return &KeyHandler{svc: svc, logger: logger}
}

// Create handles POST /v1/keys.
func (h *KeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	cred := middleware.GetCredential(r.Context())

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	newCred, rawKey, err := h.svc.Create(ctx, cred.TenantID, body.Name)
	if err != nil {
		h.logger.ErrorContext(ctx, "key create failed", "err", err, "request_id", reqID)
		response.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create API key", reqID)
		return
	}

	// Raw key shown ONLY at creation. Never stored, never logged.
	response.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":         newCred.ID,
		"key_prefix": newCred.KeyPrefix,
		"raw_key":    rawKey, // shown once
		"status":     newCred.Status,
		"created_at": newCred.CreatedAt,
	})
}

// List handles GET /v1/keys.
func (h *KeyHandler) List(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	cred := middleware.GetCredential(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	creds, err := h.svc.List(ctx, cred.TenantID)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list keys", reqID)
		return
	}

	// Never expose hashes or raw keys in list response.
	type safeKey struct {
		ID        uuid.UUID              `json:"id"`
		KeyPrefix string                 `json:"key_prefix"`
		Status    domain.CredentialStatus `json:"status"`
		CreatedAt time.Time              `json:"created_at"`
		RevokedAt *time.Time             `json:"revoked_at,omitempty"`
	}
	safe := make([]safeKey, len(creds))
	for i, c := range creds {
		safe[i] = safeKey{ID: c.ID, KeyPrefix: c.KeyPrefix, Status: c.Status, CreatedAt: c.CreatedAt, RevokedAt: c.RevokedAt}
	}
	response.WriteJSON(w, http.StatusOK, map[string]any{"keys": safe})
}

// Rotate handles POST /v1/keys/{id}/rotate.
func (h *KeyHandler) Rotate(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	cred := middleware.GetCredential(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	newCred, rawKey, err := h.svc.Rotate(ctx, cred.TenantID, id)
	if err != nil {
		if isNotFound(err) {
			response.WriteError(w, http.StatusNotFound, "not_found", "Key not found", reqID)
			return
		}
		response.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to rotate key", reqID)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]any{
		"id":         newCred.ID,
		"key_prefix": newCred.KeyPrefix,
		"raw_key":    rawKey,
		"status":     newCred.Status,
		"created_at": newCred.CreatedAt,
	})
}

// Revoke handles DELETE /v1/keys/{id}.
func (h *KeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	cred := middleware.GetCredential(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid key ID", reqID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.svc.Revoke(ctx, cred.TenantID, id); err != nil {
		if isNotFound(err) {
			response.WriteError(w, http.StatusNotFound, "not_found", "Key not found", reqID)
			return
		}
		response.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to revoke key", reqID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
