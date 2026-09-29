package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/quotaforge/quotaforge/internal/observability"
	"github.com/quotaforge/quotaforge/internal/transport/http/middleware"
)

// Router wires all routes and middleware.
type Router struct {
	logger  *slog.Logger
	metrics *observability.Metrics
}

// NewRouter creates and configures the chi router.
func NewRouter(
	logger *slog.Logger,
	metrics *observability.Metrics,
	healthHandler *HealthHandler,
	decisionHandler *DecisionHandler,
	policyHandler *PolicyHandler,
	keyHandler *KeyHandler,
	assignmentHandler *AssignmentHandler,
	authMW func(http.Handler) http.Handler,
	promReg *prometheus.Registry,
) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestSize(1 << 20)) // 1 MB body limit
	r.Use(middleware.RequestID)
	r.Use(requestLogger(logger))

	// Health (no auth required)
	r.Get("/health/live", healthHandler.Live)
	r.Get("/health/ready", healthHandler.Ready)

	// Metrics
	r.Handle("/metrics", promhttp.HandlerFor(promReg, promhttp.HandlerOpts{}))

	// Authenticated API
	r.Route("/v1", func(r chi.Router) {
		r.Use(authMW)

		// Rate-limit decisions
		r.Post("/decisions", decisionHandler.Handle)

		// Policy management
		r.Route("/policies", func(r chi.Router) {
			r.Post("/", policyHandler.Create)
			r.Get("/", policyHandler.List)
			r.Get("/{id}", policyHandler.Get)
			r.Patch("/{id}", policyHandler.Update)
			r.Delete("/{id}", policyHandler.Delete)
		})

		// API key management
		r.Route("/keys", func(r chi.Router) {
			r.Post("/", keyHandler.Create)
			r.Get("/", keyHandler.List)
			r.Post("/{id}/rotate", keyHandler.Rotate)
			r.Delete("/{id}", keyHandler.Revoke)
		})

		// Client assignment
		r.Post("/assignments", assignmentHandler.Assign)
	})

	return r
}

// requestLogger logs request method, path, status, and duration.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)
			logger.InfoContext(r.Context(), "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetRequestID(r.Context()),
			)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

// Ensure responseWriter satisfies http.Flusher if underlying does.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}


// newContext adds a deadline to the context.
func newContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}
