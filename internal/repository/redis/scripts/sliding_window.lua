-- Sliding Window rate limiter (Lua script for Redis)
-- Uses a sorted set where each member is a unique "{now_ms}:{nonce}" string
-- and the score is the event timestamp in milliseconds.
--
-- KEYS[1]: sorted-set key  e.g. "rl:{tenant}:{policy}:{client}"
-- ARGV[1]: limit           (integer, max events in window)
-- ARGV[2]: window_ms       (integer, window size in milliseconds)
-- ARGV[3]: cost            (integer, events to add if allowed)
-- ARGV[4]: nonce           (string, unique per request to avoid member collisions)
--
-- Returns: {allowed, count_after, oldest_score_ms}
--   allowed      : 1 if allowed, 0 if denied
--   count_after  : number of events in window after decision
--   oldest_score : score (ms) of the oldest event in window (or 0)

local key       = KEYS[1]
local limit     = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local cost      = tonumber(ARGV[3])
local nonce     = ARGV[4]

-- Use Redis server time
local time  = redis.call('TIME')
local now_ms = tonumber(time[1]) * 1000 + math.floor(tonumber(time[2]) / 1000)
local cutoff = now_ms - window_ms

-- Remove expired events
redis.call('ZREMRANGEBYSCORE', key, '-inf', cutoff)

-- Count current events
local count = tonumber(redis.call('ZCARD', key))

local allowed      = 0
local count_after  = count
local oldest       = 0

if count + cost <= limit then
    -- Add unique members for this cost (one per unit, with nonce to avoid collision)
    for i = 1, cost do
        local member = tostring(now_ms) .. ':' .. nonce .. ':' .. tostring(i)
        redis.call('ZADD', key, now_ms, member)
    end
    count_after = count + cost
    allowed = 1
end
-- else: denied — do NOT add members (rule: denied calls must not add events)

-- Set TTL to window so memory is reclaimed automatically
redis.call('PEXPIRE', key, window_ms + 1000)

-- Get oldest score for reset calculation
local oldest_member = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
if #oldest_member >= 2 then
    oldest = tonumber(oldest_member[2])
end

return {allowed, count_after, oldest}
