package scripts

// BalanceScripts 余额相关 Lua 脚本
var BalanceScripts = struct {
	// Freeze 冻结余额
	// KEYS[1]: 可用余额键
	// KEYS[2]: 冻结余额键
	// ARGV[1]: 冻结金额 (字符串格式的数值)
	// 返回: 1 成功, 0 余额不足, -1 参数错误
	Freeze string

	// Unfreeze 解冻余额
	// KEYS[1]: 可用余额键
	// KEYS[2]: 冻结余额键
	// ARGV[1]: 解冻金额
	// 返回: 1 成功, 0 冻结余额不足, -1 参数错误
	Unfreeze string

	// Deduct 从冻结余额中扣减
	// KEYS[1]: 冻结余额键
	// ARGV[1]: 扣减金额
	// 返回: 1 成功, 0 冻结余额不足, -1 参数错误
	Deduct string

	// DeductAvailable 从可用余额中直接扣减 (不经过冻结)
	// KEYS[1]: 可用余额键
	// ARGV[1]: 扣减金额
	// 返回: 1 成功, 0 余额不足, -1 参数错误
	DeductAvailable string

	// Transfer 原子转账
	// KEYS[1]: 源账户可用余额键
	// KEYS[2]: 目标账户可用余额键
	// ARGV[1]: 转账金额
	// 返回: 1 成功, 0 源余额不足, -1 参数错误
	Transfer string

	// FreezeAndDeduct 冻结并扣减 (一步完成)
	// KEYS[1]: 可用余额键
	// ARGV[1]: 扣减金额
	// 返回: 1 成功, 0 余额不足, -1 参数错误
	FreezeAndDeduct string

	// BatchFreeze 批量冻结
	// KEYS: [available_key_1, frozen_key_1, available_key_2, frozen_key_2, ...]
	// ARGV: [amount_1, amount_2, ...]
	// 返回: 成功冻结的数量
	BatchFreeze string

	// GetBalance 获取余额信息
	// KEYS[1]: 可用余额键
	// KEYS[2]: 冻结余额键
	// 返回: [可用余额, 冻结余额]
	GetBalance string

	// IncrBalance 增加可用余额
	// KEYS[1]: 可用余额键
	// ARGV[1]: 增加金额
	// 返回: 新的可用余额
	IncrBalance string

	// CompareAndDeduct 比较并扣减
	// KEYS[1]: 可用余额键
	// ARGV[1]: 期望的最小余额
	// ARGV[2]: 扣减金额
	// 返回: 1 成功, 0 余额不足, -1 参数错误
	CompareAndDeduct string
}{
	Freeze: `
local available = redis.call('GET', KEYS[1])
if not available then
    available = '0'
end

local amount = ARGV[1]
if not amount or amount == '' then
    return -1
end

-- 使用字符串比较进行精确的数值比较
-- 注意: 这里假设金额都是整数或者固定精度的字符串
local availableNum = tonumber(available)
local amountNum = tonumber(amount)

if not availableNum or not amountNum then
    return -1
end

if amountNum <= 0 then
    return -1
end

if availableNum < amountNum then
    return 0
end

-- 扣减可用余额
local newAvailable = availableNum - amountNum
redis.call('SET', KEYS[1], tostring(newAvailable))

-- 增加冻结余额
local frozen = redis.call('GET', KEYS[2])
if not frozen then
    frozen = '0'
end
local frozenNum = tonumber(frozen) or 0
local newFrozen = frozenNum + amountNum
redis.call('SET', KEYS[2], tostring(newFrozen))

return 1
`,

	Unfreeze: `
local frozen = redis.call('GET', KEYS[2])
if not frozen then
    frozen = '0'
end

local amount = ARGV[1]
if not amount or amount == '' then
    return -1
end

local frozenNum = tonumber(frozen)
local amountNum = tonumber(amount)

if not frozenNum or not amountNum then
    return -1
end

if amountNum <= 0 then
    return -1
end

if frozenNum < amountNum then
    return 0
end

-- 扣减冻结余额
local newFrozen = frozenNum - amountNum
redis.call('SET', KEYS[2], tostring(newFrozen))

-- 增加可用余额
local available = redis.call('GET', KEYS[1])
if not available then
    available = '0'
end
local availableNum = tonumber(available) or 0
local newAvailable = availableNum + amountNum
redis.call('SET', KEYS[1], tostring(newAvailable))

return 1
`,

	Deduct: `
local frozen = redis.call('GET', KEYS[1])
if not frozen then
    frozen = '0'
end

local amount = ARGV[1]
if not amount or amount == '' then
    return -1
end

local frozenNum = tonumber(frozen)
local amountNum = tonumber(amount)

if not frozenNum or not amountNum then
    return -1
end

if amountNum <= 0 then
    return -1
end

if frozenNum < amountNum then
    return 0
end

-- 扣减冻结余额
local newFrozen = frozenNum - amountNum
redis.call('SET', KEYS[1], tostring(newFrozen))

return 1
`,

	DeductAvailable: `
local available = redis.call('GET', KEYS[1])
if not available then
    available = '0'
end

local amount = ARGV[1]
if not amount or amount == '' then
    return -1
end

local availableNum = tonumber(available)
local amountNum = tonumber(amount)

if not availableNum or not amountNum then
    return -1
end

if amountNum <= 0 then
    return -1
end

if availableNum < amountNum then
    return 0
end

-- 扣减可用余额
local newAvailable = availableNum - amountNum
redis.call('SET', KEYS[1], tostring(newAvailable))

return 1
`,

	Transfer: `
local srcAvailable = redis.call('GET', KEYS[1])
if not srcAvailable then
    srcAvailable = '0'
end

local amount = ARGV[1]
if not amount or amount == '' then
    return -1
end

local srcAvailableNum = tonumber(srcAvailable)
local amountNum = tonumber(amount)

if not srcAvailableNum or not amountNum then
    return -1
end

if amountNum <= 0 then
    return -1
end

if srcAvailableNum < amountNum then
    return 0
end

-- 扣减源账户
local newSrcAvailable = srcAvailableNum - amountNum
redis.call('SET', KEYS[1], tostring(newSrcAvailable))

-- 增加目标账户
local dstAvailable = redis.call('GET', KEYS[2])
if not dstAvailable then
    dstAvailable = '0'
end
local dstAvailableNum = tonumber(dstAvailable) or 0
local newDstAvailable = dstAvailableNum + amountNum
redis.call('SET', KEYS[2], tostring(newDstAvailable))

return 1
`,

	FreezeAndDeduct: `
local available = redis.call('GET', KEYS[1])
if not available then
    available = '0'
end

local amount = ARGV[1]
if not amount or amount == '' then
    return -1
end

local availableNum = tonumber(available)
local amountNum = tonumber(amount)

if not availableNum or not amountNum then
    return -1
end

if amountNum <= 0 then
    return -1
end

if availableNum < amountNum then
    return 0
end

-- 直接扣减可用余额 (相当于冻结后立即扣减)
local newAvailable = availableNum - amountNum
redis.call('SET', KEYS[1], tostring(newAvailable))

return 1
`,

	BatchFreeze: `
local successCount = 0
local keyCount = #KEYS / 2

for i = 1, keyCount do
    local availableKey = KEYS[i * 2 - 1]
    local frozenKey = KEYS[i * 2]
    local amount = ARGV[i]

    if amount and amount ~= '' then
        local available = redis.call('GET', availableKey)
        if not available then
            available = '0'
        end

        local availableNum = tonumber(available)
        local amountNum = tonumber(amount)

        if availableNum and amountNum and amountNum > 0 and availableNum >= amountNum then
            -- 扣减可用余额
            local newAvailable = availableNum - amountNum
            redis.call('SET', availableKey, tostring(newAvailable))

            -- 增加冻结余额
            local frozen = redis.call('GET', frozenKey)
            if not frozen then
                frozen = '0'
            end
            local frozenNum = tonumber(frozen) or 0
            local newFrozen = frozenNum + amountNum
            redis.call('SET', frozenKey, tostring(newFrozen))

            successCount = successCount + 1
        end
    end
end

return successCount
`,

	GetBalance: `
local available = redis.call('GET', KEYS[1])
if not available then
    available = '0'
end

local frozen = redis.call('GET', KEYS[2])
if not frozen then
    frozen = '0'
end

return {available, frozen}
`,

	IncrBalance: `
local available = redis.call('GET', KEYS[1])
if not available then
    available = '0'
end

local amount = ARGV[1]
if not amount or amount == '' then
    return -1
end

local availableNum = tonumber(available)
local amountNum = tonumber(amount)

if not availableNum or not amountNum then
    return -1
end

local newAvailable = availableNum + amountNum
redis.call('SET', KEYS[1], tostring(newAvailable))

return tostring(newAvailable)
`,

	CompareAndDeduct: `
local available = redis.call('GET', KEYS[1])
if not available then
    available = '0'
end

local minBalance = ARGV[1]
local amount = ARGV[2]

if not minBalance or minBalance == '' or not amount or amount == '' then
    return -1
end

local availableNum = tonumber(available)
local minBalanceNum = tonumber(minBalance)
local amountNum = tonumber(amount)

if not availableNum or not minBalanceNum or not amountNum then
    return -1
end

if amountNum <= 0 then
    return -1
end

-- 检查余额是否大于等于期望的最小值加上扣减金额
if availableNum < minBalanceNum + amountNum then
    return 0
end

-- 扣减可用余额
local newAvailable = availableNum - amountNum
redis.call('SET', KEYS[1], tostring(newAvailable))

return 1
`,
}
