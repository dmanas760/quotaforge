-- Token Bucket rate limiter (Lua script for Redis)
-- KEYS[1]: namespaced hash key  e.g. "rl:{tenant}:{policy}:{client}"
-- ARGV[1]: capacity       (integer, max tokens)
-- ARGV[2]: refill_rate_ms (float, tokens per millisecond = rate/1000)
-- ARGV[3]: cost           (integer, tokens to consume)
-- ARGV[4]: expiry_ms      (integer, key TTL in milliseconds)
--
-- Returns a three-element array: {allowed, tokens_after, reset_ms}
--   allowed    : 1 if request is allowed, 0 if denied
--   tokens_after: remaining tokens after decision (integer, floored)
--   reset_ms   : Unix milliseconds when bucket will be fully refilled

local key          = KEYS[1]
local capacity     = tonumber(ARGV[1])
local rate_per_ms  = tonumber(ARGV[2])
local cost         = tonumber(ARGV[3])
local expiry_ms    = tonumber(ARGV[4])

-- Use Redis server time (seconds, microseconds) to avoid clock skew.
local time         = redis.call('TIME')
local now_ms       = tonumber(time[1]) * 1000 + math.floor(tonumber(time[2]) / 1000)

local tokens       = tonumber(redis.call('HGET', key, 'tokens'))
local last_ms      = tonumber(redis.call('HGET', key, 'last_refill_ms'))

if tokens == nil or last_ms == nil then
    -- First request: full bucket
    tokens  = capacity
    last_ms = now_ms
end

-- Refill: add elapsed * rate, capped at capacity.
local elapsed = math.max(0, now_ms - last_ms)
tokens = math.min(capacity, tokens + elapsed * rate_per_ms)

local allowed         = 0
local tokens_after    = math.floor(tokens)
local full_refill_ms  = math.ceil((capacity - tokens) / rate_per_ms)
local reset_ms        = now_ms + full_refill_ms

if tokens >= cost then
    tokens_after = math.floor(tokens - cost)
    allowed      = 1
    -- Update last_refill_ms only when we consume, so future refill is correct.
    redis.call('HSET', key, 'tokens', tokens_after, 'last_refill_ms', now_ms)
    -- TTL = time to fully refill remaining tokens, with a safety margin.
    local ttl = math.ceil(math.max(expiry_ms, (capacity - tokens_after) / rate_per_ms + 1000))
    redis.call('PEXPIRE', key, ttl)
    -- Recalculate reset for response.
    reset_ms = now_ms + math.ceil((capacity - tokens_after) / rate_per_ms)
else
    -- Denied: do NOT update state (rule: denied calls must not consume tokens).
    redis.call('HSET', key, 'tokens', tokens_after, 'last_refill_ms', now_ms)
    redis.call('PEXPIRE', key, expiry_ms)
end

return {allowed, tokens_after, reset_ms}
