package app

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	goredis "github.com/redis/go-redis/v9"
)

// Dependencies holds wired application dependencies.
type Dependencies struct {
	Pool        *pgxpool.Pool
	RedisClient *goredis.Client
}

// NewPostgresPool creates and validates a pgx connection pool.
func NewPostgresPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("creating postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}
	return pool, nil
}

// NewRedisClient creates and pings a Redis client.
func NewRedisClient(ctx context.Context, addr, password string, db int) (*goredis.Client, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("pinging redis: %w", err)
	}
	return client, nil
}

// RunMigrations applies pending goose migrations.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	db := stdlib.OpenDBFromPool(pool)

	goose.SetBaseFS(nil)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("setting goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}

// PingPostgres health-checks PostgreSQL.
func PingPostgres(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, "SELECT 1")
	return err
}

// PingRedis health-checks Redis.
func PingRedis(ctx context.Context, client *goredis.Client) error {
	return client.Ping(ctx).Err()
}
