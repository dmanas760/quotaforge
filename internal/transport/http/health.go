package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/quotaforge/quotaforge/internal/transport/http/response"
)

// HealthDependencyChecker checks external dependencies for readiness.
type HealthDependencyChecker interface {
	PingPostgres(ctx context.Context) error
	PingRedis(ctx context.Context) error
}

// HealthHandler handles liveness and readiness probes.
type HealthHandler struct {
	deps   HealthDependencyChecker
	logger *slog.Logger
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(deps HealthDependencyChecker, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{deps: deps, logger: logger}
}

// Live handles GET /health/live — always returns 200 if the process is up.
func (h *HealthHandler) Live(w http.ResponseWriter, r *http.Request) {
	response.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready handles GET /health/ready — returns 200 only when external deps are healthy.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	type check struct {
		name string
		fn   func(context.Context) error
	}

	checks := []check{
		{"postgres", h.deps.PingPostgres},
		{"redis", h.deps.PingRedis},
	}

	issues := make(map[string]string)
	for _, c := range checks {
		if err := c.fn(ctx); err != nil {
			issues[c.name] = err.Error()
			h.logger.WarnContext(ctx, "readiness check failed", "dep", c.name, "err", err)
		}
	}

	if len(issues) > 0 {
		response.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready",
			"checks": issues,
		})
		return
	}
	response.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
