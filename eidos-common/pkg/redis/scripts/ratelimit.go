package scripts

// RateLimitScripts 限流相关 Lua 脚本
var RateLimitScripts = struct {
	// FixedWindow 固定窗口限流
	// KEYS[1]: 限流键
	// ARGV[1]: 窗口大小 (秒)
	// ARGV[2]: 最大请求数
	// 返回: 1 允许, 0 拒绝
	FixedWindow string

	// FixedWindowWithInfo 固定窗口限流 (带信息)
	// KEYS[1]: 限流键
	// ARGV[1]: 窗口大小 (秒)
	// ARGV[2]: 最大请求数
	// 返回: [是否允许(1/0), 当前计数, 剩余配额, TTL]
	FixedWindowWithInfo string

	// SlidingWindow 滑动窗口限流
	// KEYS[1]: 限流键
	// ARGV[1]: 窗口大小 (毫秒)
	// ARGV[2]: 最大请求数
	// ARGV[3]: 当前时间戳 (毫秒)
	// 返回: 1 允许, 0 拒绝
	SlidingWindow string

	// SlidingWindowWithInfo 滑动窗口限流 (带信息)
	// KEYS[1]: 限流键
	// ARGV[1]: 窗口大小 (毫秒)
	// ARGV[2]: 最大请求数
	// ARGV[3]: 当前时间戳 (毫秒)
	// 返回: [是否允许(1/0), 当前计数, 剩余配额]
	SlidingWindowWithInfo string

	// TokenBucket 令牌桶限流
	// KEYS[1]: 令牌桶键
	// ARGV[1]: 桶容量
	// ARGV[2]: 每秒生成令牌数
	// ARGV[3]: 当前时间戳 (毫秒)
	// ARGV[4]: 请求的令牌数 (默认 1)
	// 返回: 1 允许, 0 拒绝
	TokenBucket string

	// TokenBucketWithInfo 令牌桶限流 (带信息)
	// KEYS[1]: 令牌桶键
	// ARGV[1]: 桶容量
	// ARGV[2]: 每秒生成令牌数
	// ARGV[3]: 当前时间戳 (毫秒)
	// ARGV[4]: 请求的令牌数 (默认 1)
	// 返回: [是否允许(1/0), 当前令牌数, 下次可用时间]
	TokenBucketWithInfo string

	// LeakyBucket 漏桶限流
	// KEYS[1]: 漏桶键
	// ARGV[1]: 桶容量
	// ARGV[2]: 每秒漏出速率
	// ARGV[3]: 当前时间戳 (毫秒)
	// 返回: 1 允许, 0 拒绝
	LeakyBucket string

	// LeakyBucketWithInfo 漏桶限流 (带信息)
	// KEYS[1]: 漏桶键
	// ARGV[1]: 桶容量
	// ARGV[2]: 每秒漏出速率
	// ARGV[3]: 当前时间戳 (毫秒)
	// 返回: [是否允许(1/0), 当前水位, 预计等待时间(毫秒)]
	LeakyBucketWithInfo string

	// ConcurrencyLimit 并发限制
	// KEYS[1]: 并发键
	// ARGV[1]: 最大并发数
	// ARGV[2]: 请求标识
	// ARGV[3]: 超时时间 (毫秒)
	// 返回: 1 获取成功, 0 达到限制
	ConcurrencyLimit string

	// ConcurrencyRelease 释放并发
	// KEYS[1]: 并发键
	// ARGV[1]: 请求标识
	// 返回: 释放后的并发数
	ConcurrencyRelease string

	// QuotaLimit 配额限制 (按时间周期)
	// KEYS[1]: 配额键
	// ARGV[1]: 周期 (秒)
	// ARGV[2]: 配额上限
	// ARGV[3]: 本次消耗量
	// 返回: 1 允许, 0 配额不足
	QuotaLimit string

	// QuotaLimitWithInfo 配额限制 (带信息)
	// KEYS[1]: 配额键
	// ARGV[1]: 周期 (秒)
	// ARGV[2]: 配额上限
	// ARGV[3]: 本次消耗量
	// 返回: [是否允许(1/0), 已使用配额, 剩余配额, TTL]
	QuotaLimitWithInfo string
}{
	FixedWindow: `
local current = redis.call('INCR', KEYS[1])
if current == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end

local limit = tonumber(ARGV[2])
if current > limit then
    return 0
end

return 1
`,

	FixedWindowWithInfo: `
local current = redis.call('INCR', KEYS[1])
if current == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end

local limit = tonumber(ARGV[2])
local ttl = redis.call('TTL', KEYS[1])
local remaining = limit - current

if remaining < 0 then
    remaining = 0
end

if current > limit then
    return {0, current, remaining, ttl}
end

return {1, current, remaining, ttl}
`,

	SlidingWindow: `
local key = KEYS[1]
local windowMs = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

-- 清理过期的记录
local windowStart = now - windowMs
redis.call('ZREMRANGEBYSCORE', key, '-inf', windowStart)

-- 获取当前窗口内的请求数
local current = redis.call('ZCARD', key)

if current >= limit then
    return 0
end

-- 添加新请求
redis.call('ZADD', key, now, now .. ':' .. math.random())
redis.call('PEXPIRE', key, windowMs)

return 1
`,

	SlidingWindowWithInfo: `
local key = KEYS[1]
local windowMs = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

-- 清理过期的记录
local windowStart = now - windowMs
redis.call('ZREMRANGEBYSCORE', key, '-inf', windowStart)

-- 获取当前窗口内的请求数
local current = redis.call('ZCARD', key)
local remaining = limit - current

if remaining < 0 then
    remaining = 0
end

if current >= limit then
    return {0, current, remaining}
end

-- 添加新请求
redis.call('ZADD', key, now, now .. ':' .. math.random())
redis.call('PEXPIRE', key, windowMs)

return {1, current + 1, remaining - 1}
`,

	TokenBucket: `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4]) or 1

-- 获取当前状态
local bucket = redis.call('HMGET', key, 'tokens', 'last_time')
local tokens = tonumber(bucket[1])
local lastTime = tonumber(bucket[2])

-- 初始化
if not tokens then
    tokens = capacity
    lastTime = now
end

-- 计算新生成的令牌
local elapsed = (now - lastTime) / 1000  -- 转换为秒
local newTokens = elapsed * rate
tokens = math.min(capacity, tokens + newTokens)

-- 检查是否有足够的令牌
if tokens < requested then
    -- 更新状态
    redis.call('HMSET', key, 'tokens', tokens, 'last_time', now)
    redis.call('PEXPIRE', key, math.ceil(capacity / rate * 1000) + 1000)
    return 0
end

-- 消耗令牌
tokens = tokens - requested
redis.call('HMSET', key, 'tokens', tokens, 'last_time', now)
redis.call('PEXPIRE', key, math.ceil(capacity / rate * 1000) + 1000)

return 1
`,

	TokenBucketWithInfo: `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4]) or 1

-- 获取当前状态
local bucket = redis.call('HMGET', key, 'tokens', 'last_time')
local tokens = tonumber(bucket[1])
local lastTime = tonumber(bucket[2])

-- 初始化
if not tokens then
    tokens = capacity
    lastTime = now
end

-- 计算新生成的令牌
local elapsed = (now - lastTime) / 1000
local newTokens = elapsed * rate
tokens = math.min(capacity, tokens + newTokens)

-- 计算下次可用时间
local nextAvailable = 0
if tokens < requested then
    local needed = requested - tokens
    nextAvailable = math.ceil(needed / rate * 1000)
end

-- 检查是否有足够的令牌
if tokens < requested then
    redis.call('HMSET', key, 'tokens', tokens, 'last_time', now)
    redis.call('PEXPIRE', key, math.ceil(capacity / rate * 1000) + 1000)
    return {0, tokens, nextAvailable}
end

-- 消耗令牌
tokens = tokens - requested
redis.call('HMSET', key, 'tokens', tokens, 'last_time', now)
redis.call('PEXPIRE', key, math.ceil(capacity / rate * 1000) + 1000)

return {1, tokens, 0}
`,

	LeakyBucket: `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

-- 获取当前状态
local bucket = redis.call('HMGET', key, 'water', 'last_time')
local water = tonumber(bucket[1]) or 0
local lastTime = tonumber(bucket[2]) or now

-- 计算漏出的水量
local elapsed = (now - lastTime) / 1000
local leaked = elapsed * rate
water = math.max(0, water - leaked)

-- 检查是否能加入新水滴
if water >= capacity then
    redis.call('HMSET', key, 'water', water, 'last_time', now)
    redis.call('PEXPIRE', key, math.ceil(capacity / rate * 1000) + 1000)
    return 0
end

-- 加入新水滴
water = water + 1
redis.call('HMSET', key, 'water', water, 'last_time', now)
redis.call('PEXPIRE', key, math.ceil(capacity / rate * 1000) + 1000)

return 1
`,

	LeakyBucketWithInfo: `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

-- 获取当前状态
local bucket = redis.call('HMGET', key, 'water', 'last_time')
local water = tonumber(bucket[1]) or 0
local lastTime = tonumber(bucket[2]) or now

-- 计算漏出的水量
local elapsed = (now - lastTime) / 1000
local leaked = elapsed * rate
water = math.max(0, water - leaked)

-- 计算等待时间
local waitTime = 0
if water >= capacity then
    waitTime = math.ceil((water - capacity + 1) / rate * 1000)
end

-- 检查是否能加入新水滴
if water >= capacity then
    redis.call('HMSET', key, 'water', water, 'last_time', now)
    redis.call('PEXPIRE', key, math.ceil(capacity / rate * 1000) + 1000)
    return {0, water, waitTime}
end

-- 加入新水滴
water = water + 1
redis.call('HMSET', key, 'water', water, 'last_time', now)
redis.call('PEXPIRE', key, math.ceil(capacity / rate * 1000) + 1000)

return {1, water, 0}
`,

	ConcurrencyLimit: `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local requestId = ARGV[2]
local timeoutMs = tonumber(ARGV[3])
local now = redis.call('TIME')
local nowMs = tonumber(now[1]) * 1000 + tonumber(now[2]) / 1000

-- 清理过期的并发记录
redis.call('ZREMRANGEBYSCORE', key, '-inf', nowMs - timeoutMs)

-- 检查当前并发数
local current = redis.call('ZCARD', key)
if current >= limit then
    return 0
end

-- 添加新的并发记录
redis.call('ZADD', key, nowMs + timeoutMs, requestId)
redis.call('PEXPIRE', key, timeoutMs + 1000)

return 1
`,

	ConcurrencyRelease: `
redis.call('ZREM', KEYS[1], ARGV[1])
return redis.call('ZCARD', KEYS[1])
`,

	QuotaLimit: `
local key = KEYS[1]
local period = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local cost = tonumber(ARGV[3])

-- 获取当前使用量
local current = tonumber(redis.call('GET', key)) or 0

-- 检查配额
if current + cost > limit then
    return 0
end

-- 增加使用量
local newValue = redis.call('INCRBY', key, cost)
if newValue == cost then
    -- 第一次设置，添加过期时间
    redis.call('EXPIRE', key, period)
end

return 1
`,

	QuotaLimitWithInfo: `
local key = KEYS[1]
local period = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local cost = tonumber(ARGV[3])

-- 获取当前使用量
local current = tonumber(redis.call('GET', key)) or 0
local ttl = redis.call('TTL', key)
if ttl < 0 then
    ttl = period
end

-- 检查配额
if current + cost > limit then
    return {0, current, limit - current, ttl}
end

-- 增加使用量
local newValue = redis.call('INCRBY', key, cost)
if newValue == cost then
    redis.call('EXPIRE', key, period)
    ttl = period
end

local remaining = limit - newValue
if remaining < 0 then
    remaining = 0
end

return {1, newValue, remaining, ttl}
`,
}
