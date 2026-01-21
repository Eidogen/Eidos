package scripts

// CommonScripts 通用 Lua 脚本
var CommonScripts = struct {
	// SetIfGreater 如果新值大于当前值则设置
	// KEYS[1]: 键
	// ARGV[1]: 新值
	// 返回: 1 已更新, 0 未更新
	SetIfGreater string

	// SetIfLess 如果新值小于当前值则设置
	// KEYS[1]: 键
	// ARGV[1]: 新值
	// 返回: 1 已更新, 0 未更新
	SetIfLess string

	// GetAndDelete 获取并删除
	// KEYS[1]: 键
	// 返回: 值 (如果存在)
	GetAndDelete string

	// GetMultipleWithDefault 批量获取 (带默认值)
	// KEYS: 所有键
	// ARGV[1]: 默认值
	// 返回: 值列表
	GetMultipleWithDefault string

	// IncrByWithCap 递增但不超过上限
	// KEYS[1]: 键
	// ARGV[1]: 递增量
	// ARGV[2]: 上限
	// 返回: 新值
	IncrByWithCap string

	// DecrByWithFloor 递减但不低于下限
	// KEYS[1]: 键
	// ARGV[1]: 递减量
	// ARGV[2]: 下限
	// 返回: 新值
	DecrByWithFloor string

	// CompareAndSet CAS 操作
	// KEYS[1]: 键
	// ARGV[1]: 期望值
	// ARGV[2]: 新值
	// 返回: 1 成功, 0 值不匹配
	CompareAndSet string

	// CompareAndDelete 比较并删除
	// KEYS[1]: 键
	// ARGV[1]: 期望值
	// 返回: 1 成功, 0 值不匹配
	CompareAndDelete string

	// MultiSet 批量设置 (原子操作)
	// KEYS: 所有键
	// ARGV: 对应的值
	// 返回: 设置的键数量
	MultiSet string

	// ExpireMultiple 批量设置过期时间
	// KEYS: 所有键
	// ARGV[1]: 过期时间 (秒)
	// 返回: 成功设置的数量
	ExpireMultiple string

	// HashIncrByMultiple 批量递增哈希字段
	// KEYS[1]: 哈希键
	// ARGV: [field1, incr1, field2, incr2, ...]
	// 返回: 新值列表
	HashIncrByMultiple string

	// ListTrim 列表修剪并返回被删除的元素
	// KEYS[1]: 列表键
	// ARGV[1]: 保留开始位置
	// ARGV[2]: 保留结束位置
	// 返回: 被删除的元素列表
	ListTrim string

	// SetCounter 原子计数器 (带过期和上限)
	// KEYS[1]: 计数器键
	// ARGV[1]: 递增量
	// ARGV[2]: 过期时间 (秒)
	// ARGV[3]: 上限 (可选)
	// 返回: 新值
	SetCounter string

	// ZSetAddWithCap 有序集合添加并限制大小
	// KEYS[1]: 有序集合键
	// ARGV[1]: 分数
	// ARGV[2]: 成员
	// ARGV[3]: 最大大小
	// 返回: 添加后的集合大小
	ZSetAddWithCap string
}{
	SetIfGreater: `
local current = tonumber(redis.call('GET', KEYS[1]))
local newValue = tonumber(ARGV[1])

if not current or newValue > current then
    redis.call('SET', KEYS[1], ARGV[1])
    return 1
end

return 0
`,

	SetIfLess: `
local current = tonumber(redis.call('GET', KEYS[1]))
local newValue = tonumber(ARGV[1])

if not current or newValue < current then
    redis.call('SET', KEYS[1], ARGV[1])
    return 1
end

return 0
`,

	GetAndDelete: `
local value = redis.call('GET', KEYS[1])
if value then
    redis.call('DEL', KEYS[1])
end
return value
`,

	GetMultipleWithDefault: `
local results = {}
local default = ARGV[1]

for i, key in ipairs(KEYS) do
    local value = redis.call('GET', key)
    if value then
        results[i] = value
    else
        results[i] = default
    end
end

return results
`,

	IncrByWithCap: `
local current = tonumber(redis.call('GET', KEYS[1])) or 0
local incr = tonumber(ARGV[1])
local cap = tonumber(ARGV[2])

local newValue = current + incr
if newValue > cap then
    newValue = cap
end

redis.call('SET', KEYS[1], tostring(newValue))
return newValue
`,

	DecrByWithFloor: `
local current = tonumber(redis.call('GET', KEYS[1])) or 0
local decr = tonumber(ARGV[1])
local floor = tonumber(ARGV[2])

local newValue = current - decr
if newValue < floor then
    newValue = floor
end

redis.call('SET', KEYS[1], tostring(newValue))
return newValue
`,

	CompareAndSet: `
local current = redis.call('GET', KEYS[1])
local expected = ARGV[1]
local newValue = ARGV[2]

if current == expected then
    redis.call('SET', KEYS[1], newValue)
    return 1
end

return 0
`,

	CompareAndDelete: `
local current = redis.call('GET', KEYS[1])
local expected = ARGV[1]

if current == expected then
    redis.call('DEL', KEYS[1])
    return 1
end

return 0
`,

	MultiSet: `
local count = 0
for i, key in ipairs(KEYS) do
    if ARGV[i] then
        redis.call('SET', key, ARGV[i])
        count = count + 1
    end
end
return count
`,

	ExpireMultiple: `
local expire = tonumber(ARGV[1])
local count = 0

for _, key in ipairs(KEYS) do
    if redis.call('EXPIRE', key, expire) == 1 then
        count = count + 1
    end
end

return count
`,

	HashIncrByMultiple: `
local key = KEYS[1]
local results = {}
local argCount = #ARGV

for i = 1, argCount, 2 do
    local field = ARGV[i]
    local incr = tonumber(ARGV[i + 1]) or 0
    local newValue = redis.call('HINCRBY', key, field, incr)
    table.insert(results, newValue)
end

return results
`,

	ListTrim: `
local key = KEYS[1]
local start = tonumber(ARGV[1])
local stop = tonumber(ARGV[2])

-- 获取将被删除的元素
local len = redis.call('LLEN', key)
local removed = {}

-- 获取前面将被删除的元素
if start > 0 then
    local frontRemoved = redis.call('LRANGE', key, 0, start - 1)
    for _, v in ipairs(frontRemoved) do
        table.insert(removed, v)
    end
end

-- 获取后面将被删除的元素
if stop < len - 1 then
    local backRemoved = redis.call('LRANGE', key, stop + 1, -1)
    for _, v in ipairs(backRemoved) do
        table.insert(removed, v)
    end
end

-- 执行修剪
redis.call('LTRIM', key, start, stop)

return removed
`,

	SetCounter: `
local key = KEYS[1]
local incr = tonumber(ARGV[1])
local expire = tonumber(ARGV[2])
local cap = tonumber(ARGV[3])

local newValue = redis.call('INCRBY', key, incr)

-- 应用上限
if cap and newValue > cap then
    newValue = cap
    redis.call('SET', key, tostring(newValue))
end

-- 设置过期时间
redis.call('EXPIRE', key, expire)

return newValue
`,

	ZSetAddWithCap: `
local key = KEYS[1]
local score = tonumber(ARGV[1])
local member = ARGV[2]
local maxSize = tonumber(ARGV[3])

-- 添加成员
redis.call('ZADD', key, score, member)

-- 获取当前大小
local size = redis.call('ZCARD', key)

-- 如果超出上限，移除最低分数的成员
if size > maxSize then
    local removeCount = size - maxSize
    redis.call('ZREMRANGEBYRANK', key, 0, removeCount - 1)
    size = maxSize
end

return size
`,
}

// IdempotentScripts 幂等性相关脚本
var IdempotentScripts = struct {
	// CheckAndSet 检查幂等键是否存在，不存在则设置
	// KEYS[1]: 幂等键
	// ARGV[1]: 值
	// ARGV[2]: 过期时间 (秒)
	// 返回: 1 首次执行, 0 重复执行
	CheckAndSet string

	// CheckAndSetWithResult 检查幂等键并存储结果
	// KEYS[1]: 幂等键
	// ARGV[1]: 结果值
	// ARGV[2]: 过期时间 (秒)
	// 返回: [是否首次(1/0), 已存储的结果]
	CheckAndSetWithResult string

	// GetIdempotentResult 获取幂等操作结果
	// KEYS[1]: 幂等键
	// 返回: 存储的结果
	GetIdempotentResult string
}{
	CheckAndSet: `
if redis.call('SET', KEYS[1], ARGV[1], 'NX', 'EX', ARGV[2]) then
    return 1
else
    return 0
end
`,

	CheckAndSetWithResult: `
local exists = redis.call('GET', KEYS[1])
if exists then
    return {0, exists}
end

redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
return {1, ARGV[1]}
`,

	GetIdempotentResult: `
return redis.call('GET', KEYS[1])
`,
}
