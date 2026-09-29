package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/quotaforge/quotaforge/internal/domain"
	httputil "github.com/quotaforge/quotaforge/internal/transport/http/response"
)

type credentialLooker interface {
	GetByPrefix(ctx context.Context, keyPrefix string) (*domain.Credential, error)
	VerifyKey(rawKey string, hash string) bool
}

// Auth authenticates requests using a Bearer service API key.
// Keys are stored as Argon2id hashes; the raw key is NEVER logged.
func Auth(repo credentialLooker, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawKey := extractBearerToken(r)
			if rawKey == "" {
				httputil.WriteError(w, http.StatusUnauthorized, "missing_api_key", "Authorization header with Bearer token required", GetRequestID(r.Context()))
				return
			}

			// Keys have a non-secret prefix (first 8 chars) for lookup.
			if len(rawKey) < 8 {
				httputil.WriteError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid API key format", GetRequestID(r.Context()))
				return
			}
			prefix := rawKey[:8]

			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()

			cred, err := repo.GetByPrefix(ctx, prefix)
			if err != nil {
				if errors.As(err, &domain.ErrNotFound{}) {
					httputil.WriteError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid or revoked API key", GetRequestID(r.Context()))
					return
				}
				logger.ErrorContext(ctx, "credential lookup failed", "err", err, "request_id", GetRequestID(ctx))
				httputil.WriteError(w, http.StatusServiceUnavailable, "dependency_unavailable", "Service temporarily unavailable", GetRequestID(r.Context()))
				return
			}

			if cred.Status == domain.CredentialStatusRevoked {
				httputil.WriteError(w, http.StatusUnauthorized, "revoked_api_key", "API key has been revoked", GetRequestID(r.Context()))
				return
			}

			if !repo.VerifyKey(rawKey, cred.KeyHash) {
				httputil.WriteError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid or revoked API key", GetRequestID(r.Context()))
				return
			}

			// Attach credential to context
			ctx = withCredential(r.Context(), cred)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

const credentialKey contextKey = "credential"

func withCredential(ctx context.Context, c *domain.Credential) context.Context {
	return context.WithValue(ctx, credentialKey, c)
}

// GetCredential extracts the authenticated credential from context.
func GetCredential(ctx context.Context) *domain.Credential {
	c, _ := ctx.Value(credentialKey).(*domain.Credential)
	return c
}
