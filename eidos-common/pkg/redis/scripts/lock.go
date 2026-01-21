package scripts

// LockScripts 分布式锁相关 Lua 脚本
var LockScripts = struct {
	// Acquire 获取锁
	// KEYS[1]: 锁键
	// ARGV[1]: 锁值 (唯一标识)
	// ARGV[2]: 过期时间 (毫秒)
	// 返回: 1 成功, 0 失败
	Acquire string

	// Release 释放锁
	// KEYS[1]: 锁键
	// ARGV[1]: 锁值
	// 返回: 1 成功, 0 锁不存在或不匹配
	Release string

	// Extend 延长锁过期时间
	// KEYS[1]: 锁键
	// ARGV[1]: 锁值
	// ARGV[2]: 新的过期时间 (毫秒)
	// 返回: 1 成功, 0 锁不存在或不匹配
	Extend string

	// AcquireWithWatchdog 获取锁并设置看门狗续期
	// KEYS[1]: 锁键
	// KEYS[2]: 看门狗键
	// ARGV[1]: 锁值
	// ARGV[2]: 过期时间 (毫秒)
	// 返回: 1 成功, 0 失败
	AcquireWithWatchdog string

	// ReleaseWithWatchdog 释放锁并清除看门狗
	// KEYS[1]: 锁键
	// KEYS[2]: 看门狗键
	// ARGV[1]: 锁值
	// 返回: 1 成功, 0 锁不存在或不匹配
	ReleaseWithWatchdog string

	// ReentrantAcquire 可重入锁获取
	// KEYS[1]: 锁键
	// ARGV[1]: 锁值 (线程/协程标识)
	// ARGV[2]: 过期时间 (毫秒)
	// 返回: 当前重入次数, 0 表示获取失败
	ReentrantAcquire string

	// ReentrantRelease 可重入锁释放
	// KEYS[1]: 锁键
	// ARGV[1]: 锁值
	// 返回: 剩余重入次数, -1 表示完全释放, 0 表示锁不存在或不匹配
	ReentrantRelease string

	// TryAcquireWithQueue 带排队的锁获取
	// KEYS[1]: 锁键
	// KEYS[2]: 队列键
	// ARGV[1]: 锁值
	// ARGV[2]: 过期时间 (毫秒)
	// ARGV[3]: 当前时间戳
	// 返回: 1 获取成功, 0 排队中 (返回队列位置), -1 失败
	TryAcquireWithQueue string

	// GetLockInfo 获取锁信息
	// KEYS[1]: 锁键
	// 返回: [锁值, 剩余过期时间(毫秒)]
	GetLockInfo string

	// ForceRelease 强制释放锁 (管理员操作)
	// KEYS[1]: 锁键
	// 返回: 1 成功, 0 锁不存在
	ForceRelease string
}{
	Acquire: `
if redis.call('SET', KEYS[1], ARGV[1], 'NX', 'PX', ARGV[2]) then
    return 1
else
    return 0
end
`,

	Release: `
if redis.call('GET', KEYS[1]) == ARGV[1] then
    return redis.call('DEL', KEYS[1])
else
    return 0
end
`,

	Extend: `
if redis.call('GET', KEYS[1]) == ARGV[1] then
    return redis.call('PEXPIRE', KEYS[1], ARGV[2])
else
    return 0
end
`,

	AcquireWithWatchdog: `
if redis.call('SET', KEYS[1], ARGV[1], 'NX', 'PX', ARGV[2]) then
    -- 设置看门狗标记
    redis.call('SET', KEYS[2], '1', 'PX', ARGV[2])
    return 1
else
    return 0
end
`,

	ReleaseWithWatchdog: `
if redis.call('GET', KEYS[1]) == ARGV[1] then
    redis.call('DEL', KEYS[1])
    redis.call('DEL', KEYS[2])
    return 1
else
    return 0
end
`,

	ReentrantAcquire: `
local lockValue = redis.call('GET', KEYS[1])

if not lockValue then
    -- 锁不存在，直接获取
    redis.call('SET', KEYS[1], ARGV[1] .. ':1', 'PX', ARGV[2])
    return 1
end

-- 解析当前锁值
local sep = string.find(lockValue, ':')
if not sep then
    return 0
end

local currentOwner = string.sub(lockValue, 1, sep - 1)
local currentCount = tonumber(string.sub(lockValue, sep + 1))

if currentOwner == ARGV[1] then
    -- 同一个持有者，增加重入次数
    local newCount = currentCount + 1
    redis.call('SET', KEYS[1], ARGV[1] .. ':' .. tostring(newCount), 'PX', ARGV[2])
    return newCount
else
    -- 不是同一个持有者
    return 0
end
`,

	ReentrantRelease: `
local lockValue = redis.call('GET', KEYS[1])

if not lockValue then
    return 0
end

-- 解析当前锁值
local sep = string.find(lockValue, ':')
if not sep then
    return 0
end

local currentOwner = string.sub(lockValue, 1, sep - 1)
local currentCount = tonumber(string.sub(lockValue, sep + 1))

if currentOwner ~= ARGV[1] then
    return 0
end

if currentCount > 1 then
    -- 减少重入次数
    local newCount = currentCount - 1
    local ttl = redis.call('PTTL', KEYS[1])
    if ttl > 0 then
        redis.call('SET', KEYS[1], ARGV[1] .. ':' .. tostring(newCount), 'PX', ttl)
    end
    return newCount
else
    -- 完全释放
    redis.call('DEL', KEYS[1])
    return -1
end
`,

	TryAcquireWithQueue: `
local lockKey = KEYS[1]
local queueKey = KEYS[2]
local lockValue = ARGV[1]
local expireMs = ARGV[2]
local now = tonumber(ARGV[3])

-- 清理过期的排队请求 (超过 30 秒)
redis.call('ZREMRANGEBYSCORE', queueKey, '-inf', now - 30000)

-- 检查是否已持有锁
local currentLock = redis.call('GET', lockKey)
if currentLock == lockValue then
    -- 已持有，续期
    redis.call('PEXPIRE', lockKey, expireMs)
    return 1
end

-- 检查锁是否空闲
if not currentLock then
    -- 检查队列中是否有等待者
    local firstInQueue = redis.call('ZRANGE', queueKey, 0, 0)
    if #firstInQueue == 0 or firstInQueue[1] == lockValue then
        -- 队列为空或者自己是第一个，获取锁
        redis.call('SET', lockKey, lockValue, 'PX', expireMs)
        redis.call('ZREM', queueKey, lockValue)
        return 1
    end
end

-- 加入排队
local score = redis.call('ZSCORE', queueKey, lockValue)
if not score then
    redis.call('ZADD', queueKey, now, lockValue)
end

-- 返回队列位置
local rank = redis.call('ZRANK', queueKey, lockValue)
return 0 - rank  -- 返回负数表示排队位置
`,

	GetLockInfo: `
local lockValue = redis.call('GET', KEYS[1])
if not lockValue then
    return nil
end

local ttl = redis.call('PTTL', KEYS[1])
return {lockValue, ttl}
`,

	ForceRelease: `
local result = redis.call('DEL', KEYS[1])
return result
`,
}

// ReadWriteLockScripts 读写锁相关 Lua 脚本
var ReadWriteLockScripts = struct {
	// AcquireRead 获取读锁
	// KEYS[1]: 读锁计数键
	// KEYS[2]: 写锁键
	// ARGV[1]: 读者标识
	// ARGV[2]: 过期时间 (毫秒)
	// 返回: 1 成功, 0 失败 (写锁被持有)
	AcquireRead string

	// ReleaseRead 释放读锁
	// KEYS[1]: 读锁计数键
	// ARGV[1]: 读者标识
	// 返回: 剩余读者数量
	ReleaseRead string

	// AcquireWrite 获取写锁
	// KEYS[1]: 读锁计数键
	// KEYS[2]: 写锁键
	// ARGV[1]: 写者标识
	// ARGV[2]: 过期时间 (毫秒)
	// 返回: 1 成功, 0 失败
	AcquireWrite string

	// ReleaseWrite 释放写锁
	// KEYS[1]: 写锁键
	// ARGV[1]: 写者标识
	// 返回: 1 成功, 0 失败
	ReleaseWrite string

	// GetLockStatus 获取锁状态
	// KEYS[1]: 读锁计数键
	// KEYS[2]: 写锁键
	// 返回: [读者数量, 写锁持有者]
	GetLockStatus string
}{
	AcquireRead: `
-- 检查是否有写锁
local writeLock = redis.call('GET', KEYS[2])
if writeLock then
    return 0
end

-- 增加读者计数
redis.call('HINCRBY', KEYS[1], ARGV[1], 1)
redis.call('PEXPIRE', KEYS[1], ARGV[2])

return 1
`,

	ReleaseRead: `
local count = redis.call('HINCRBY', KEYS[1], ARGV[1], -1)
if count <= 0 then
    redis.call('HDEL', KEYS[1], ARGV[1])
end

local remaining = redis.call('HLEN', KEYS[1])
if remaining == 0 then
    redis.call('DEL', KEYS[1])
end

return remaining
`,

	AcquireWrite: `
-- 检查是否有读者
local readerCount = redis.call('HLEN', KEYS[1])
if readerCount > 0 then
    return 0
end

-- 尝试获取写锁
if redis.call('SET', KEYS[2], ARGV[1], 'NX', 'PX', ARGV[2]) then
    return 1
else
    return 0
end
`,

	ReleaseWrite: `
if redis.call('GET', KEYS[2]) == ARGV[1] then
    return redis.call('DEL', KEYS[2])
else
    return 0
end
`,

	GetLockStatus: `
local readerCount = redis.call('HLEN', KEYS[1])
local writeLock = redis.call('GET', KEYS[2])

if not writeLock then
    writeLock = ''
end

return {readerCount, writeLock}
`,
}
