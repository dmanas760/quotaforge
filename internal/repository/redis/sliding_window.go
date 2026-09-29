package redis

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"fmt"

	"github.com/quotaforge/quotaforge/internal/domain"
	goredis "github.com/redis/go-redis/v9"
)

//go:embed scripts/sliding_window.lua
var slidingWindowScript string

// SlidingWindowLimiter implements domain.Limiter using a Redis Lua script.
type SlidingWindowLimiter struct {
	client *goredis.Client
	sha    string
}

// NewSlidingWindowLimiter loads the Lua script into Redis.
func NewSlidingWindowLimiter(ctx context.Context, client *goredis.Client) (*SlidingWindowLimiter, error) {
	sha, err := client.ScriptLoad(ctx, slidingWindowScript).Result()
	if err != nil {
		return nil, fmt.Errorf("loading sliding_window script: %w", err)
	}
	return &SlidingWindowLimiter{client: client, sha: sha}, nil
}

// Check atomically executes the sliding-window script.
func (l *SlidingWindowLimiter) Check(ctx context.Context, key domain.LimiterKey, policy *domain.Policy, cost int64) (*domain.LimiterResult, error) {
	windowMs := policy.WindowSeconds * 1000
	nonce := newNonce()

	vals, err := l.evalScript(ctx, key.String(), policy.Capacity, windowMs, cost, nonce)
	if err != nil {
		return nil, fmt.Errorf("sliding_window eval: %w", err)
	}

	// reset = oldest event time + window_ms
	var resetMs int64
	if vals[2] > 0 {
		resetMs = vals[2] + windowMs
	}

	return &domain.LimiterResult{
		Allowed:   vals[0] == 1,
		Remaining: policy.Capacity - vals[1],
		ResetAtMs: resetMs,
	}, nil
}

func (l *SlidingWindowLimiter) evalScript(ctx context.Context, key string, limit, windowMs, cost int64, nonce string) ([]int64, error) {
	vals, err := l.client.EvalSha(ctx, l.sha, []string{key},
		limit, windowMs, cost, nonce,
	).Int64Slice()
	if err != nil && isNOSCRIPT(err) {
		vals, err = l.client.Eval(ctx, slidingWindowScript, []string{key},
			limit, windowMs, cost, nonce,
		).Int64Slice()
	}
	if err != nil {
		return nil, err
	}
	if len(vals) < 3 {
		return nil, fmt.Errorf("unexpected sliding_window result length: %d", len(vals))
	}
	return vals, nil
}

func newNonce() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
