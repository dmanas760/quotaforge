package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	goredis "github.com/redis/go-redis/v9"
	"github.com/quotaforge/quotaforge/internal/app"
	"github.com/quotaforge/quotaforge/internal/config"
	"github.com/quotaforge/quotaforge/internal/observability"
	"github.com/quotaforge/quotaforge/internal/repository/postgres"
	redisrepo "github.com/quotaforge/quotaforge/internal/repository/redis"
	"github.com/quotaforge/quotaforge/internal/service"
	httphandler "github.com/quotaforge/quotaforge/internal/transport/http"
	"github.com/quotaforge/quotaforge/internal/transport/http/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("loading config", "err", err)
		os.Exit(1)
	}

	logger := observability.NewLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ----- Infrastructure -----------------------------------------------

	pool, err := app.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connecting to postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := app.RunMigrations(ctx, pool, "migrations"); err != nil {
		logger.Error("running migrations", "err", err)
		os.Exit(1)
	}

	redisClient, err := app.NewRedisClient(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		logger.Error("connecting to redis", "err", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	// ----- Observability --------------------------------------------------

	promReg := prometheus.NewRegistry()
	metrics := observability.NewMetrics(promReg)

	// ----- Repositories ---------------------------------------------------

	policyRepo := postgres.NewPolicyRepo(pool)
	credRepo := postgres.NewCredentialRepo(pool)
	auditRepo := postgres.NewAuditRepo(pool)
	assignRepo := postgres.NewAssignmentRepo(pool)

	// ----- Redis limiters ------------------------------------------------

	tbLimiter, err := redisrepo.NewTokenBucketLimiter(ctx, redisClient)
	if err != nil {
		logger.Error("loading token_bucket script", "err", err)
		os.Exit(1)
	}

	swLimiter, err := redisrepo.NewSlidingWindowLimiter(ctx, redisClient)
	if err != nil {
		logger.Error("loading sliding_window script", "err", err)
		os.Exit(1)
	}

	// ----- Services ------------------------------------------------------

	credSvc := service.NewCredentialService(credRepo, auditRepo,
		cfg.ArgonTime, cfg.ArgonMemory, cfg.ArgonThreads, cfg.ArgonKeyLen)

	policySvc := service.NewPolicyService(policyRepo, auditRepo, metrics)
	decisionSvc := service.NewDecisionService(policySvc, assignRepo, tbLimiter, swLimiter, metrics)

	// ----- Health checker -------------------------------------------------

	healthDeps := &healthChecker{pool: pool, redis: redisClient}

	// ----- HTTP handlers --------------------------------------------------

	healthHandler := httphandler.NewHealthHandler(healthDeps, logger)
	decisionHandler := httphandler.NewDecisionHandler(decisionSvc, logger)
	policyHandler := httphandler.NewPolicyHandler(policySvc, logger)
	keyHandler := httphandler.NewKeyHandler(credSvc, logger)
	assignmentHandler := httphandler.NewAssignmentHandler(decisionSvc, logger)

	authMW := middleware.Auth(credSvc, logger)

	router := httphandler.NewRouter(
		logger, metrics,
		healthHandler, decisionHandler, policyHandler, keyHandler, assignmentHandler,
		authMW, promReg,
	)

	// ----- HTTP server ---------------------------------------------------

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("QuotaForge API starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	// Wait for signal
	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}
	logger.Info("server stopped")
}

// healthChecker adapts app-level ping functions to the HealthDependencyChecker interface.
type healthChecker struct {
	pool  *pgxpool.Pool
	redis *goredis.Client
}

func (h *healthChecker) PingPostgres(ctx context.Context) error {
	_, err := h.pool.Exec(ctx, "SELECT 1")
	return err
}

func (h *healthChecker) PingRedis(ctx context.Context) error {
	return h.redis.Ping(ctx).Err()
}
