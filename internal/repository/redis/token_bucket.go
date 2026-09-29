package redis

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/quotaforge/quotaforge/internal/domain"
	goredis "github.com/redis/go-redis/v9"
)

//go:embed scripts/token_bucket.lua
var tokenBucketScript string

// TokenBucketLimiter implements domain.Limiter using a Redis Lua script.
type TokenBucketLimiter struct {
	client *goredis.Client
	sha    string
}

// NewTokenBucketLimiter loads the Lua script into Redis and caches its SHA.
func NewTokenBucketLimiter(ctx context.Context, client *goredis.Client) (*TokenBucketLimiter, error) {
	sha, err := client.ScriptLoad(ctx, tokenBucketScript).Result()
	if err != nil {
		return nil, fmt.Errorf("loading token_bucket script: %w", err)
	}
	return &TokenBucketLimiter{client: client, sha: sha}, nil
}

// Check atomically executes the token-bucket script.
func (l *TokenBucketLimiter) Check(ctx context.Context, key domain.LimiterKey, policy *domain.Policy, cost int64) (*domain.LimiterResult, error) {
	// refill_rate_ms = rate per second / 1000
	ratePerMs := policy.RefillRate / 1000.0
	// expiry = time to fully refill from empty
	expiryMs := int64(float64(policy.Capacity)/ratePerMs) + 60_000

	result, err := l.evalScript(ctx, key.String(), policy.Capacity, ratePerMs, cost, expiryMs)
	if err != nil {
		return nil, fmt.Errorf("token_bucket eval: %w", err)
	}
	return result, nil
}

func (l *TokenBucketLimiter) evalScript(ctx context.Context, key string, capacity int64, ratePerMs float64, cost, expiryMs int64) (*domain.LimiterResult, error) {
	vals, err := l.client.EvalSha(ctx, l.sha, []string{key},
		capacity, ratePerMs, cost, expiryMs,
	).Int64Slice()

	// If SHA not loaded (e.g., Redis restart), fall back to EVAL.
	if err != nil && isNOSCRIPT(err) {
		vals, err = l.client.Eval(ctx, tokenBucketScript, []string{key},
			capacity, ratePerMs, cost, expiryMs,
		).Int64Slice()
	}
	if err != nil {
		return nil, err
	}
	if len(vals) < 3 {
		return nil, fmt.Errorf("unexpected token_bucket result length: %d", len(vals))
	}

	return &domain.LimiterResult{
		Allowed:   vals[0] == 1,
		Remaining: vals[1],
		ResetAtMs: vals[2],
	}, nil
}

func isNOSCRIPT(err error) bool {
	if err == nil {
		return false
	}
	return len(err.Error()) >= 8 && err.Error()[:8] == "NOSCRIPT"
}

// resetTime converts Unix milliseconds to time.Time.
func resetTime(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}
