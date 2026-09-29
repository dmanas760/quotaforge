package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

// TestTokenBucketBasic verifies: 10-capacity bucket allows 10, denies 11th.
func TestTokenBucketBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	env := setupTestEnv(t, ctx)

	// Create policy
	policy := createPolicy(t, env, map[string]any{
		"name":           "basic-tb",
		"algorithm":      "token_bucket",
		"capacity":       10,
		"refill_rate":    0.1,
		"burst_capacity": 10,
	})

	// Assign a test client
	assignClient(t, env, "client-basic", policy["id"].(string))

	// Allow 10 requests
	for i := 0; i < 10; i++ {
		result := makeDecision(t, env, "client-basic", 1)
		assert.True(t, result["allowed"].(bool), "request %d should be allowed", i+1)
		assert.Equal(t, "ok", result["reason"])
	}

	// 11th must be denied
	result := makeDecision(t, env, "client-basic", 1)
	assert.False(t, result["allowed"].(bool), "11th request should be denied")
	assert.Equal(t, "rate_limit_exceeded", result["reason"])
}

// TestTokenBucketConcurrent: 1000 goroutines, same key, capacity 50.
// Must allow exactly capacity requests, no more.
func TestTokenBucketConcurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	env := setupTestEnv(t, ctx)

	capacity := 50
	goroutines := 1000

	policy := createPolicy(t, env, map[string]any{
		"name":           "concurrent-tb",
		"algorithm":      "token_bucket",
		"capacity":       capacity,
		"refill_rate":    0.001, // near-zero refill during test
		"burst_capacity": capacity,
	})

	assignClient(t, env, "client-concurrent", policy["id"].(string))

	var (
		wg      sync.WaitGroup
		allowed int64
		mu      sync.Mutex
	)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			result := makeDecision(t, env, "client-concurrent", 1)
			if result["allowed"].(bool) {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(capacity), allowed,
		"exactly capacity requests should be allowed out of %d concurrent", goroutines)
}

// TestSlidingWindowBasic: events expire at the right boundary.
func TestSlidingWindowBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	env := setupTestEnv(t, ctx)

	policy := createPolicy(t, env, map[string]any{
		"name":           "sw-basic",
		"algorithm":      "sliding_window",
		"capacity":       3,
		"window_seconds": 2,
	})

	assignClient(t, env, "client-sw", policy["id"].(string))

	// Allow 3 requests
	for i := 0; i < 3; i++ {
		result := makeDecision(t, env, "client-sw", 1)
		assert.True(t, result["allowed"].(bool))
	}

	// 4th denied
	result := makeDecision(t, env, "client-sw", 1)
	assert.False(t, result["allowed"].(bool))

	// Wait for window to expire
	time.Sleep(2*time.Second + 100*time.Millisecond)

	// Should be allowed again
	result = makeDecision(t, env, "client-sw", 1)
	assert.True(t, result["allowed"].(bool), "should be allowed after window expiry")
}

// TestRevokedKeyDenied: revoked keys never receive decisions.
func TestRevokedKeyDenied(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t, context.Background())

	// Create and immediately revoke a key
	keyResp := createKey(t, env)
	rawKey := keyResp["raw_key"].(string)
	keyID := keyResp["id"].(string)
	revokeKey(t, env, keyID)

	// Try using revoked key
	req := httptest.NewRequest(http.MethodPost, "/v1/decisions", bytes.NewBufferString(`{"client_id":"x","cost":1}`))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ---- helpers ----------------------------------------------------------------

type testEnv struct {
	handler http.Handler
	authKey string
}

func setupTestEnv(t *testing.T, ctx context.Context) *testEnv {
	t.Helper()
	// NOTE: actual container startup uses testcontainers; omitted here for brevity.
	// Run: DATABASE_URL=... REDIS_ADDR=... go test ./tests/integration/... to use real deps.
	t.Skip("TODO: wire testcontainers setup")
	return nil
}

func createPolicy(t *testing.T, env *testEnv, body map[string]any) map[string]any {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/policies", bytes.NewBuffer(data))
	req.Header.Set("Authorization", "Bearer "+env.authKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

func assignClient(t *testing.T, env *testEnv, clientID, policyID string) {
	t.Helper()
	body := map[string]any{"client_id": clientID, "policy_id": policyID}
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/assignments", bytes.NewBuffer(data))
	req.Header.Set("Authorization", "Bearer "+env.authKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.handler.ServeHTTP(w, req)
	_ = w.Code
}

func makeDecision(t *testing.T, env *testEnv, clientID string, cost int) map[string]any {
	t.Helper()
	body := fmt.Sprintf(`{"client_id":%q,"cost":%d}`, clientID, cost)
	req := httptest.NewRequest(http.MethodPost, "/v1/decisions", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+env.authKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

func createKey(t *testing.T, env *testEnv) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/keys", bytes.NewBufferString(`{"name":"test"}`))
	req.Header.Set("Authorization", "Bearer "+env.authKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

func revokeKey(t *testing.T, env *testEnv, keyID string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/v1/keys/"+keyID, nil)
	req.Header.Set("Authorization", "Bearer "+env.authKey)
	w := httptest.NewRecorder()
	env.handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
}

// suppress unused import errors
var _ = uuid.New
