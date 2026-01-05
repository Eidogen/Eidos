package cache

import "github.com/redis/go-redis/v9"

var (
	// luaGetOrCreateBalance 原子获取或创建余额
	// KEYS[1]: balance_key (e.g., "eidos:trading:balance:0x123:USDC")
	// ARGV[1]: current_timestamp (ms)
	// Returns: HGETALL result of the balance hash
	luaGetOrCreateBalance = redis.NewScript(`
		local key = KEYS[1]
		local exists = redis.call('EXISTS', key)
		if exists == 1 then
			return redis.call('HGETALL', key)
		end
		redis.call('HSET', key,
			'settled_available', '0',
			'settled_frozen', '0',
			'pending_available', '0',
			'pending_frozen', '0',
			'pending_total', '0',
			'version', '1',
			'updated_at', ARGV[1]
		)
		return redis.call('HGETALL', key)
	`)

	// luaFreeze 原子冻结余额
	// KEYS[1]: balance_key
	// ARGV[1]: available_field ("settled_available" or "pending_available")
	// ARGV[2]: frozen_field ("settled_frozen" or "pending_frozen")
	// ARGV[3]: amount to freeze
	// ARGV[4]: current_timestamp
	// Returns: {ok: "success"} or {err: "error_message"}
	luaFreeze = redis.NewScript(`
		local key = KEYS[1]
		local available_field = ARGV[1]
		local frozen_field = ARGV[2]
		local amount = ARGV[3]
		local now = ARGV[4]

		local available = redis.call('HGET', key, available_field)
		if not available then
			return {err = 'balance not found'}
		end

		available = tonumber(available)
		local freeze_amount = tonumber(amount)

		if available < freeze_amount then
			return {err = 'insufficient balance'}
		end

		redis.call('HINCRBYFLOAT', key, available_field, -freeze_amount)
		redis.call('HINCRBYFLOAT', key, frozen_field, freeze_amount)
		redis.call('HINCRBY', key, 'version', 1)
		redis.call('HSET', key, 'updated_at', now)

		return {ok = 'success'}
	`)

	// luaFreezeForOrder 原子冻结余额并记录订单冻结信息 (下单核心逻辑)
	// 这是一个复杂的原子操作，包含：
	// 1. 幂等性检查 (Nonce)
	// 2. 风控检查 (MaxOpenOrders, PendingLimit)
	// 3. 余额初始化 (如果不存在)
	// 4. 资金冻结 (优先扣减已结算，不足部分扣减待结算)
	// 5. 记录订单冻结明细 (OrderFreezeRecord)
	// 6. 写入 Transactional Outbox (保证消息不丢失)
	// KEYS[1]: balance_key
	// KEYS[2]: order_freeze_key
	// KEYS[3]: nonce_key
	// KEYS[4]: outbox_key
	// KEYS[5]: outbox_pending_list_key
	// KEYS[6]: user_pending_limit_key
	// KEYS[7]: global_pending_limit_key
	// KEYS[8]: user_open_orders_key
	// ARGV...: wallet, token, amount, order_id, from_settled, order_json, etc.
	luaFreezeForOrder = redis.NewScript(`
		local balance_key = KEYS[1]
		local order_freeze_key = KEYS[2]
		local nonce_key = KEYS[3]
		local outbox_key = KEYS[4]
		local outbox_pending_key = KEYS[5]
		local user_pending_key = KEYS[6]
		local global_pending_key = KEYS[7]
		local user_open_orders_key = KEYS[8]

		local wallet = ARGV[1]
		local token = ARGV[2]
		local amount = tonumber(ARGV[3])
		local order_id = ARGV[4]
		local from_settled = ARGV[5] == '1'
		local order_json = ARGV[6]
		local shard_id = ARGV[7]
		local nonce_ttl = tonumber(ARGV[8])
		local now = ARGV[9]
		local user_pending_limit = tonumber(ARGV[10])
		local global_pending_limit = tonumber(ARGV[11])
		local max_open_orders = tonumber(ARGV[12])

		-- 1. 检查 Nonce 是否已使用
		if redis.call('EXISTS', nonce_key) == 1 then
			return {'err', 'nonce_used'}
		end

		-- 2. 检查用户活跃订单数 (如果设置了限制)
		if max_open_orders > 0 then
			local current_open_orders = tonumber(redis.call('GET', user_open_orders_key) or '0')
			if current_open_orders >= max_open_orders then
				return {'err', 'max_open_orders_exceeded'}
			end
		end

		-- 3. 检查余额
		local exists = redis.call('EXISTS', balance_key)
		if exists == 0 then
			-- 创建新余额
			redis.call('HSET', balance_key,
				'settled_available', '0',
				'settled_frozen', '0',
				'pending_available', '0',
				'pending_frozen', '0',
				'pending_total', '0',
				'version', '1',
				'updated_at', now
			)
		end

		local settled_available = tonumber(redis.call('HGET', balance_key, 'settled_available') or '0')
		local pending_available = tonumber(redis.call('HGET', balance_key, 'pending_available') or '0')
		local pending_frozen = tonumber(redis.call('HGET', balance_key, 'pending_frozen') or '0')

		-- 4. 计算冻结来源
		local settled_freeze = 0
		local pending_freeze = 0

		if from_settled then
			-- 优先从已结算冻结
			if settled_available >= amount then
				settled_freeze = amount
			elseif settled_available + pending_available >= amount then
				settled_freeze = settled_available
				pending_freeze = amount - settled_available
			else
				return {'err', 'insufficient_balance'}
			end
		else
			-- 优先从待结算冻结
			if pending_available >= amount then
				pending_freeze = amount
			elseif settled_available + pending_available >= amount then
				pending_freeze = pending_available
				settled_freeze = amount - pending_available
			else
				return {'err', 'insufficient_balance'}
			end
		end

		-- 5. 待结算限额检查 (如果使用了待结算余额)
		if pending_freeze > 0 then
			local pending_total = tonumber(redis.call('HGET', balance_key, 'pending_total') or '0')

			-- 5.1 用户待结算限额检查
			if user_pending_limit > 0 then
				if pending_total > user_pending_limit then
					return {'err', 'pending_limit_exceeded'}
				end
			end

			-- 5.2 全局待结算限额检查
			if global_pending_limit > 0 then
				local global_pending_total = tonumber(redis.call('GET', global_pending_key) or '0')
				if global_pending_total > global_pending_limit then
					return {'err', 'global_pending_limit_exceeded'}
				end
			end
		end

		-- 6. 执行冻结
		if settled_freeze > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'settled_available', -settled_freeze)
			redis.call('HINCRBYFLOAT', balance_key, 'settled_frozen', settled_freeze)
		end
		if pending_freeze > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'pending_available', -pending_freeze)
			redis.call('HINCRBYFLOAT', balance_key, 'pending_frozen', pending_freeze)
		end
		redis.call('HINCRBY', balance_key, 'version', 1)
		redis.call('HSET', balance_key, 'updated_at', now)

		-- 7. 记录订单冻结信息
		redis.call('HSET', order_freeze_key,
			'order_id', order_id,
			'token', token,
			'settled_amount', tostring(settled_freeze),
			'pending_amount', tostring(pending_freeze)
		)

		-- 8. 标记 Nonce 已使用
		redis.call('SETEX', nonce_key, nonce_ttl, '1')

		-- 9. 写入 Outbox
		redis.call('HSET', outbox_key,
			'status', 'PENDING',
			'order_json', order_json,
			'shard', shard_id,
			'retry_count', '0',
			'created_at', now,
			'updated_at', now
		)
		redis.call('LPUSH', outbox_pending_key, order_id)

		-- 10. 增加用户活跃订单计数
		redis.call('INCR', user_open_orders_key)

		return {'ok', 'success', tostring(settled_freeze), tostring(pending_freeze)}
	`)

	// luaUnfreeze 原子解冻余额 (将冻结金额退回可用)
	// KEYS[1]: balance_key
	// ARGV[1]: frozen_field
	// ARGV[2]: available_field
	// ARGV[3]: amount
	// ARGV[4]: current_timestamp
	luaUnfreeze = redis.NewScript(`
		local key = KEYS[1]
		local frozen_field = ARGV[1]
		local available_field = ARGV[2]
		local amount = tonumber(ARGV[3])
		local now = ARGV[4]

		local frozen = tonumber(redis.call('HGET', key, frozen_field) or '0')
		if frozen < amount then
			return {err = 'insufficient frozen balance'}
		end

		redis.call('HINCRBYFLOAT', key, frozen_field, -amount)
		redis.call('HINCRBYFLOAT', key, available_field, amount)
		redis.call('HINCRBY', key, 'version', 1)
		redis.call('HSET', key, 'updated_at', now)

		return {ok = 'success'}
	`)

	// luaUnfreezeByOrderID 根据订单ID自动查找并解冻资金
	// 用于过期、拒绝或由于系统错误需要撤单的场景
	// 它会自动读取 order_freeze_record，知道当初冻结了多少 settled 和 pending，并原样退回。
	// KEYS[1]: balance_key
	// KEYS[2]: order_freeze_key
	// ARGV[1]: current_timestamp
	luaUnfreezeByOrderID = redis.NewScript(`
		local balance_key = KEYS[1]
		local order_freeze_key = KEYS[2]
		local now = ARGV[1]

		-- 获取订单冻结记录
		local freeze_data = redis.call('HGETALL', order_freeze_key)
		if #freeze_data == 0 then
			return {err = 'order freeze not found'}
		end

		local freeze = {}
		for i = 1, #freeze_data, 2 do
			freeze[freeze_data[i]] = freeze_data[i+1]
		end

		local settled_amount = tonumber(freeze['settled_amount'] or '0')
		local pending_amount = tonumber(freeze['pending_amount'] or '0')

		-- 解冻
		if settled_amount > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'settled_frozen', -settled_amount)
			redis.call('HINCRBYFLOAT', balance_key, 'settled_available', settled_amount)
		end
		if pending_amount > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'pending_frozen', -pending_amount)
			redis.call('HINCRBYFLOAT', balance_key, 'pending_available', pending_amount)
		end

		redis.call('HINCRBY', balance_key, 'version', 1)
		redis.call('HSET', balance_key, 'updated_at', now)

		-- 删除冻结记录
		redis.call('DEL', order_freeze_key)

		return {ok = 'success'}
	`)

	// luaUnfreezeForCancel 原子解冻并写入 Cancel Outbox (已废弃，保留兼容)
	// 老版本的取消逻辑，同时做解冻和写入消息队列。
	// 新版本已拆分为 WriteCancelOutbox (只发消息) 和 HandleCancelConfirm (收到消息后解冻)。
	luaUnfreezeForCancel = redis.NewScript(`
		local balance_key = KEYS[1]
		local order_freeze_key = KEYS[2]
		local outbox_key = KEYS[3]
		local outbox_pending_key = KEYS[4]

		local wallet = ARGV[1]
		local order_id = ARGV[2]
		local cancel_json = ARGV[3]
		local shard_id = ARGV[4]
		local now = ARGV[5]

		-- 1. 幂等检查 (Outbox 是否已存在)
		if redis.call('EXISTS', outbox_key) == 1 then
			return {ok = 'already_exists'}
		end

		-- 2. 获取订单冻结记录
		local freeze_data = redis.call('HGETALL', order_freeze_key)
		if #freeze_data == 0 then
			-- 记录不存在，可能已经被撮合引擎部分成交扣除，仍然需要发送取消请求
			-- 写入 Outbox，但不解冻资金 (因为找不到冻结记录)
			redis.call('HSET', outbox_key,
				'status', 'PENDING',
				'cancel_json', cancel_json,
				'shard', shard_id,
				'retry_count', '0',
				'created_at', now,
				'updated_at', now
			)
			redis.call('LPUSH', outbox_pending_key, order_id)
			return {ok = 'success_no_freeze'}
		end
		
		local freeze = {}
		for i = 1, #freeze_data, 2 do
			freeze[freeze_data[i]] = freeze_data[i+1]
		end

		local settled_amount = tonumber(freeze['settled_amount'] or '0')
		local pending_amount = tonumber(freeze['pending_amount'] or '0')

		-- 3. 解冻余额
		if settled_amount > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'settled_frozen', -settled_amount)
			redis.call('HINCRBYFLOAT', balance_key, 'settled_available', settled_amount)
		end
		if pending_amount > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'pending_frozen', -pending_amount)
			redis.call('HINCRBYFLOAT', balance_key, 'pending_available', pending_amount)
		end

		redis.call('HINCRBY', balance_key, 'version', 1)
		redis.call('HSET', balance_key, 'updated_at', now)

		-- 4. 删除订单冻结记录
		redis.call('DEL', order_freeze_key)

		-- 5. 写入 Cancel Outbox
		redis.call('HSET', outbox_key,
			'status', 'PENDING',
			'cancel_json', cancel_json,
			'shard', shard_id,
			'retry_count', '0',
			'created_at', now,
			'updated_at', now
		)
		redis.call('LPUSH', outbox_pending_key, order_id)

		return {ok = 'success'}
	`)

	// luaWriteCancelOutbox 写入 Cancel Outbox (不解冻资金)
	// 仅用于发送取消请求到 Kafka，资金的解冻推迟到撮合引擎确认取消后。
	// KEYS[1]: outbox_key
	// KEYS[2]: outbox_pending_list_key
	// ARGV[1]: cancel_json
	// ARGV[2]: shard_id
	// ARGV[3]: current_timestamp
	// ARGV[4]: order_id
	luaWriteCancelOutbox = redis.NewScript(`
		local outbox_key = KEYS[1]
		local outbox_pending_key = KEYS[2]

		local exists = redis.call('EXISTS', outbox_key)
		if exists == 1 then
			return {ok = 'already_exists'}
		end

		redis.call('HSET', outbox_key,
			'status', 'PENDING',
			'cancel_json', ARGV[1],
			'shard', ARGV[2],
			'retry_count', '0',
			'created_at', ARGV[3],
			'updated_at', ARGV[3]
		)
		redis.call('LPUSH', outbox_pending_key, ARGV[4])
		
		return {ok = 'success'}
	`)

	// luaCancelPendingOrder 取消 PENDING 状态订单
	// 针对尚未发往撮合引擎的订单，直接在 Redis 层面拦截并回滚资金。
	// KEYS[1]: balance_key
	// KEYS[2]: order_freeze_key
	// KEYS[3]: outbox_key
	// KEYS[4]: outbox_pending_list_key
	luaCancelPendingOrder = redis.NewScript(`
		local balance_key = KEYS[1]
		local order_freeze_key = KEYS[2]
		local outbox_key = KEYS[3]
		local outbox_pending_key = KEYS[4]

		local order_id = ARGV[1]
		local cancel_json = ARGV[2]
		local shard_id = ARGV[3]
		local now = ARGV[4]

		-- 1. 检查 Outbox 状态
		-- 如果 Outbox 不存在，说明可能已经被发送或者不存在
		if redis.call('EXISTS', outbox_key) == 0 then
			return {err = 'outbox_not_found'}
		end

		local status = redis.call('HGET', outbox_key, 'status')
		if status == 'SENT' or status == 'PROCESSING' then
			-- 已经发送，无法在此处取消，需走 CancelActiveOrder 流程
			return {err = 'order_already_sent'}
		end

		-- 2. 获取订单冻结记录
		local freeze_data = redis.call('HGETALL', order_freeze_key)
		if #freeze_data == 0 then
			-- 没有冻结记录，可能已被处理，仍然写入 outbox
			redis.call('HSET', outbox_key,
				'status', 'PENDING',
				'cancel_json', cancel_json,
				'shard', shard_id,
				'retry_count', '0',
				'created_at', now,
				'updated_at', now
			)
			redis.call('LPUSH', outbox_pending_key, order_id)
			return {ok = 'success_no_freeze'}
		end

		local freeze = {}
		for i = 1, #freeze_data, 2 do
			freeze[freeze_data[i]] = freeze_data[i+1]
		end

		local settled_amount = tonumber(freeze['settled_amount'] or '0')
		local pending_amount = tonumber(freeze['pending_amount'] or '0')

		-- 3. 解冻余额
		if settled_amount > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'settled_frozen', -settled_amount)
			redis.call('HINCRBYFLOAT', balance_key, 'settled_available', settled_amount)
		end
		if pending_amount > 0 then
			redis.call('HINCRBYFLOAT', balance_key, 'pending_frozen', -pending_amount)
			redis.call('HINCRBYFLOAT', balance_key, 'pending_available', pending_amount)
		end

		redis.call('HINCRBY', balance_key, 'version', 1)
		redis.call('HSET', balance_key, 'updated_at', now)

		-- 4. 删除订单冻结记录
		redis.call('DEL', order_freeze_key)

		-- 5. 写入 Cancel Outbox
		redis.call('HSET', outbox_key,
			'status', 'PENDING',
			'cancel_json', cancel_json,
			'shard', shard_id,
			'retry_count', '0',
			'created_at', now,
			'updated_at', now
		)
		redis.call('LPUSH', outbox_pending_key, order_id)

		return {ok = 'success'}
	`)

	// luaCredit 增加余额 (充值或转入)
	// KEYS[1]: balance_key
	// ARGV[1]: field_to_increase (settled_available or pending_available)
	// ARGV[2]: amount
	// ARGV[3]: current_timestamp
	// ARGV[4]: update_pending_total_flag ('1' or '0')
	luaCredit = redis.NewScript(`
		local key = KEYS[1]
		local field = ARGV[1]
		local amount = ARGV[2]
		local now = ARGV[3]
		local update_pending_total = ARGV[4] == '1'

		-- 确保余额记录存在
		local exists = redis.call('EXISTS', key)
		if exists == 0 then
			redis.call('HSET', key,
				'settled_available', '0',
				'settled_frozen', '0',
				'pending_available', '0',
				'pending_frozen', '0',
				'pending_total', '0',
				'version', '1',
				'updated_at', now
			)
		end

		redis.call('HINCRBYFLOAT', key, field, amount)

		-- 如果是增加到 pending_available，同时增加 pending_total
		if update_pending_total then
			redis.call('HINCRBYFLOAT', key, 'pending_total', amount)
		end

		redis.call('HINCRBY', key, 'version', 1)
		redis.call('HSET', key, 'updated_at', now)

		return {ok = 'success'}
	`)

	// luaDebit 从冻结部分扣减余额 (提现或转出)
	// 注意：这里是从 *Frozen* 扣减，意味着必须先 Freeze，再 Debit。
	// 这是一个两阶段扣款模型：Freeze (预占) -> Debit (实扣)。
	luaDebit = redis.NewScript(`
		local key = KEYS[1]
		local field = ARGV[1]
		local amount = tonumber(ARGV[2])
		local now = ARGV[3]

		local frozen = tonumber(redis.call('HGET', key, field) or '0')
		if frozen < amount then
			return {err = 'insufficient frozen balance'}
		end

		redis.call('HINCRBYFLOAT', key, field, -amount)
		redis.call('HINCRBY', key, 'version', 1)
		redis.call('HSET', key, 'updated_at', now)

		return {ok = 'success'}
	`)

	// luaTransfer 原子转账 (From Wallet -> To Wallet)
	// 这是一个原子操作，保证资金不会凭空消失或增加。
	// 发送方必须先被 Freeze，从 Frozen 扣减。
	// 接收方直接增加到 Available。
	luaTransfer = redis.NewScript(`
		local from_key = KEYS[1]
		local to_key = KEYS[2]
		local frozen_field = ARGV[1]
		local amount = tonumber(ARGV[2])
		local now = ARGV[3]

		-- 1. 检查发送方冻结余额
		local frozen = tonumber(redis.call('HGET', from_key, frozen_field) or '0')
		if frozen < amount then
			return {err = 'insufficient frozen balance'}
		end

		-- 2. 从发送方冻结余额扣减
		redis.call('HINCRBYFLOAT', from_key, frozen_field, -amount)
		redis.call('HINCRBY', from_key, 'version', 1)
		redis.call('HSET', from_key, 'updated_at', now)

		-- 3. 确保接收方余额记录存在
		local to_exists = redis.call('EXISTS', to_key)
		if to_exists == 0 then
			redis.call('HSET', to_key,
				'settled_available', '0',
				'settled_frozen', '0',
				'pending_available', '0',
				'pending_frozen', '0',
				'pending_total', '0',
				'version', '1',
				'updated_at', now
			)
		end

		-- 4. 增加到接收方待结算可用，同时增加 pending_total
		redis.call('HINCRBYFLOAT', to_key, 'pending_available', amount)
		redis.call('HINCRBYFLOAT', to_key, 'pending_total', amount)
		redis.call('HINCRBY', to_key, 'version', 1)
		redis.call('HSET', to_key, 'updated_at', now)

		return {ok = 'success'}
	`)

	// luaClearTrade 清算成交 (核心逻辑)
	// 处理一笔撮合成交，涉及买卖双方的资金划转。
	// Maker (挂单方): 释放冻结 -> 扣除成交额 -> 扣除手续费 -> 获得对家资产。
	// Taker (吃单方): 同上。
	// 关键点：
	// 1. 同时更新 4 个账户 (Maker Base/Quote, Taker Base/Quote)。
	// 2. 更新 OrderFreezeRecord (因为部分成交了，冻结金额减少)。
	// 3. 记录手续费 (Fee Bucket)。
	// 4. 幂等控制 (TradeID)。
	luaClearTrade = redis.NewScript(`
		local trade_key = KEYS[1]
		local maker_base_key = KEYS[2]
		local maker_quote_key = KEYS[3]
		local taker_base_key = KEYS[4]
		local taker_quote_key = KEYS[5]
		local maker_order_freeze_key = KEYS[6]
		local taker_order_freeze_key = KEYS[7]
		local fee_bucket_key = KEYS[8]

		local base_amount = tonumber(ARGV[1])
		local quote_amount = tonumber(ARGV[2])
		local maker_fee = tonumber(ARGV[3])
		local taker_fee = tonumber(ARGV[4])
		local maker_is_buy = ARGV[5] == '1'
		local now = ARGV[6]

		-- 辅助函数: 更新订单冻结记录
		local function update_order_freeze(freeze_key, deduct_amount)
			local settled = tonumber(redis.call('HGET', freeze_key, 'settled_amount') or '0')
			local pending = tonumber(redis.call('HGET', freeze_key, 'pending_amount') or '0')
			local total = settled + pending
			if total <= 0 then return end

			-- 按比例从 settled 和 pending 扣减
			local settled_ratio = settled / total
			local settled_deduct = deduct_amount * settled_ratio
			local pending_deduct = deduct_amount - settled_deduct

			local new_settled = settled - settled_deduct
			local new_pending = pending - pending_deduct

			-- 防止浮点精度问题导致负数
			if new_settled < 0.000001 then new_settled = 0 end
			if new_pending < 0.000001 then new_pending = 0 end

			if new_settled == 0 and new_pending == 0 then
				-- 全部释放，删除记录
				redis.call('DEL', freeze_key)
			else
				redis.call('HSET', freeze_key,
					'settled_amount', tostring(new_settled),
					'pending_amount', tostring(new_pending))
			end
		end

		-- 1. 幂等检查
		if redis.call('EXISTS', trade_key) == 1 then
			return {err = 'trade_already_processed'}
		end

		-- 2. 清算逻辑
		if maker_is_buy then
			-- Maker 是买方: Maker 冻结 quote, 获得 base; Taker 冻结 base, 获得 quote
			-- Maker: 解冻并扣减 quote, 增加 base (pending_available)
			local maker_quote_frozen = tonumber(redis.call('HGET', maker_quote_key, 'settled_frozen') or '0')
			local maker_quote_pending_frozen = tonumber(redis.call('HGET', maker_quote_key, 'pending_frozen') or '0')
			local total_frozen = maker_quote_frozen + maker_quote_pending_frozen
			if total_frozen < quote_amount then
				return {err = 'maker insufficient frozen'}
			end

			-- 按比例扣减
			local settled_ratio = maker_quote_frozen / total_frozen
			local settled_deduct = quote_amount * settled_ratio
			local pending_deduct = quote_amount - settled_deduct

			local maker_receive = base_amount - maker_fee
			redis.call('HINCRBYFLOAT', maker_quote_key, 'settled_frozen', -settled_deduct)
			redis.call('HINCRBYFLOAT', maker_quote_key, 'pending_frozen', -pending_deduct)
			redis.call('HINCRBYFLOAT', maker_base_key, 'pending_available', maker_receive)
			redis.call('HINCRBYFLOAT', maker_base_key, 'pending_total', maker_receive)
			redis.call('HINCRBY', maker_quote_key, 'version', 1)
			redis.call('HINCRBY', maker_base_key, 'version', 1)
			redis.call('HSET', maker_quote_key, 'updated_at', now)
			redis.call('HSET', maker_base_key, 'updated_at', now)

			-- 更新 Maker 订单冻结记录
			update_order_freeze(maker_order_freeze_key, quote_amount)

			-- Taker: 解冻并扣减 base, 增加 quote (pending_available)
			local taker_base_frozen = tonumber(redis.call('HGET', taker_base_key, 'settled_frozen') or '0')
			local taker_base_pending_frozen = tonumber(redis.call('HGET', taker_base_key, 'pending_frozen') or '0')
			local taker_total_frozen = taker_base_frozen + taker_base_pending_frozen
			if taker_total_frozen < base_amount then
				return {err = 'taker insufficient frozen'}
			end

			local taker_settled_ratio = taker_base_frozen / taker_total_frozen
			local taker_settled_deduct = base_amount * taker_settled_ratio
			local taker_pending_deduct = base_amount - taker_settled_deduct

			local taker_receive = quote_amount - taker_fee
			redis.call('HINCRBYFLOAT', taker_base_key, 'settled_frozen', -taker_settled_deduct)
			redis.call('HINCRBYFLOAT', taker_base_key, 'pending_frozen', -taker_pending_deduct)
			redis.call('HINCRBYFLOAT', taker_quote_key, 'pending_available', taker_receive)
			redis.call('HINCRBYFLOAT', taker_quote_key, 'pending_total', taker_receive)
			redis.call('HINCRBY', taker_base_key, 'version', 1)
			redis.call('HINCRBY', taker_quote_key, 'version', 1)
			redis.call('HSET', taker_base_key, 'updated_at', now)
			redis.call('HSET', taker_quote_key, 'updated_at', now)

			-- 更新 Taker 订单冻结记录
			update_order_freeze(taker_order_freeze_key, base_amount)
		else
			-- Maker 是卖方: Maker 冻结 base, 获得 quote; Taker 冻结 quote, 获得 base
			-- Maker: 解冻并扣减 base, 增加 quote (pending_available)
			local maker_base_frozen = tonumber(redis.call('HGET', maker_base_key, 'settled_frozen') or '0')
			local maker_base_pending_frozen = tonumber(redis.call('HGET', maker_base_key, 'pending_frozen') or '0')
			local total_frozen = maker_base_frozen + maker_base_pending_frozen
			if total_frozen < base_amount then
				return {err = 'maker insufficient frozen'}
			end

			local settled_ratio = maker_base_frozen / total_frozen
			local settled_deduct = base_amount * settled_ratio
			local pending_deduct = base_amount - settled_deduct

			local maker_receive = quote_amount - maker_fee
			redis.call('HINCRBYFLOAT', maker_base_key, 'settled_frozen', -settled_deduct)
			redis.call('HINCRBYFLOAT', maker_base_key, 'pending_frozen', -pending_deduct)
			redis.call('HINCRBYFLOAT', maker_quote_key, 'pending_available', maker_receive)
			redis.call('HINCRBYFLOAT', maker_quote_key, 'pending_total', maker_receive)
			redis.call('HINCRBY', maker_base_key, 'version', 1)
			redis.call('HINCRBY', maker_quote_key, 'version', 1)
			redis.call('HSET', maker_base_key, 'updated_at', now)
			redis.call('HSET', maker_quote_key, 'updated_at', now)

			-- 更新 Maker 订单冻结记录
			update_order_freeze(maker_order_freeze_key, base_amount)

			-- Taker: 解冻并扣减 quote, 增加 base (pending_available)
			local taker_quote_frozen = tonumber(redis.call('HGET', taker_quote_key, 'settled_frozen') or '0')
			local taker_quote_pending_frozen = tonumber(redis.call('HGET', taker_quote_key, 'pending_frozen') or '0')
			local taker_total_frozen = taker_quote_frozen + taker_quote_pending_frozen
			if taker_total_frozen < quote_amount then
				return {err = 'taker insufficient frozen'}
			end

			local taker_settled_ratio = taker_quote_frozen / taker_total_frozen
			local taker_settled_deduct = quote_amount * taker_settled_ratio
			local taker_pending_deduct = quote_amount - taker_settled_deduct

			local taker_receive = base_amount - taker_fee
			redis.call('HINCRBYFLOAT', taker_quote_key, 'settled_frozen', -taker_settled_deduct)
			redis.call('HINCRBYFLOAT', taker_quote_key, 'pending_frozen', -taker_pending_deduct)
			redis.call('HINCRBYFLOAT', taker_base_key, 'pending_available', taker_receive)
			redis.call('HINCRBYFLOAT', taker_base_key, 'pending_total', taker_receive)
			redis.call('HINCRBY', taker_quote_key, 'version', 1)
			redis.call('HINCRBY', taker_base_key, 'version', 1)
			redis.call('HSET', taker_quote_key, 'updated_at', now)
			redis.call('HSET', taker_base_key, 'updated_at', now)

			-- 更新 Taker 订单冻结记录
			update_order_freeze(taker_order_freeze_key, quote_amount)
		end

		-- 3. 增加手续费分桶
		local total_fee = maker_fee + taker_fee
		if total_fee > 0 then
			redis.call('INCRBYFLOAT', fee_bucket_key, total_fee)
		end

		-- 4. 标记成交已处理 (7 天 TTL)
		redis.call('SETEX', trade_key, 604800, '1')

		return {ok = 'success'}
	`)

	// luaSettle 结算 (Pending -> Settled)
	// 将待结算资金 (Pending Available) 转换为已结算资金 (Settled Available)。
	// 通常在 T+1 结算或提现审核通过后调用。
	luaSettle = redis.NewScript(`
		local key = KEYS[1]
		local amount = tonumber(ARGV[1])
		local now = ARGV[2]

		local pending_available = tonumber(redis.call('HGET', key, 'pending_available') or '0')
		if pending_available < amount then
			return {err = 'insufficient pending balance'}
		end

		redis.call('HINCRBYFLOAT', key, 'pending_available', -amount)
		redis.call('HINCRBYFLOAT', key, 'settled_available', amount)
		redis.call('HINCRBY', key, 'version', 1)
		redis.call('HSET', key, 'updated_at', now)

		return {ok = 'success'}
	`)

	// luaRollbackTrade 回滚成交 (Saga 补偿)
	// 当后续的数据库持久化失败时，必须调用此脚本回滚 Redis 状态。
	// 它执行 ClearTrade 的反向操作：
	// 1. 从用户余额中扣回刚才赚到的钱。
	// 2. 把刚才扣掉的冻结资金退回去 (恢复冻结状态)。
	luaRollbackTrade = redis.NewScript(`
		local rollback_key = KEYS[1]
		local maker_base_key = KEYS[2]
		local maker_quote_key = KEYS[3]
		local taker_base_key = KEYS[4]
		local taker_quote_key = KEYS[5]

		local base_amount = tonumber(ARGV[1])
		local quote_amount = tonumber(ARGV[2])
		local maker_fee = tonumber(ARGV[3])
		local taker_fee = tonumber(ARGV[4])
		local maker_is_buy = ARGV[5] == '1'
		local now = ARGV[6]

		-- 1. 幂等检查
		if redis.call('EXISTS', rollback_key) == 1 then
			return {ok = 'already_rolled_back'}
		end

		-- 2. 回滚逻辑 (ClearTrade 的反向操作)
		if maker_is_buy then
			-- 原操作: Maker 扣 quote_frozen, 得 base_pending_available
			-- 回滚: Maker 扣 base_pending_available (maker_receive = base_amount - maker_fee)
			--       Maker 加 quote_pending_frozen (因为无法区分来源，统一加到 pending)
			local maker_receive = base_amount - maker_fee
			local maker_pending = tonumber(redis.call('HGET', maker_base_key, 'pending_available') or '0')

			-- 扣减 Maker 收益 (可能不足，记录实际扣减量)
			local actual_deduct = math.min(maker_pending, maker_receive)
			if actual_deduct > 0 then
				redis.call('HINCRBYFLOAT', maker_base_key, 'pending_available', -actual_deduct)
				redis.call('HINCRBYFLOAT', maker_base_key, 'pending_total', -actual_deduct)
			end
			-- 退回 Maker 冻结 (退到 pending_frozen)
			redis.call('HINCRBYFLOAT', maker_quote_key, 'pending_frozen', quote_amount)
			redis.call('HINCRBY', maker_base_key, 'version', 1)
			redis.call('HINCRBY', maker_quote_key, 'version', 1)
			redis.call('HSET', maker_base_key, 'updated_at', now)
			redis.call('HSET', maker_quote_key, 'updated_at', now)

			-- 原操作: Taker 扣 base_frozen, 得 quote_pending_available
			-- 回滚: Taker 扣 quote_pending_available (taker_receive = quote_amount - taker_fee)
			--       Taker 加 base_pending_frozen
			local taker_receive = quote_amount - taker_fee
			local taker_pending = tonumber(redis.call('HGET', taker_quote_key, 'pending_available') or '0')

			local taker_actual_deduct = math.min(taker_pending, taker_receive)
			if taker_actual_deduct > 0 then
				redis.call('HINCRBYFLOAT', taker_quote_key, 'pending_available', -taker_actual_deduct)
				redis.call('HINCRBYFLOAT', taker_quote_key, 'pending_total', -taker_actual_deduct)
			end
			redis.call('HINCRBYFLOAT', taker_base_key, 'pending_frozen', base_amount)
			redis.call('HINCRBY', taker_quote_key, 'version', 1)
			redis.call('HINCRBY', taker_base_key, 'version', 1)
			redis.call('HSET', taker_quote_key, 'updated_at', now)
			redis.call('HSET', taker_base_key, 'updated_at', now)
		else
			-- Maker 是卖方
			-- 原操作: Maker 扣 base_frozen, 得 quote_pending_available
			-- 回滚: Maker 扣 quote_pending_available, 加 base_pending_frozen
			local maker_receive = quote_amount - maker_fee
			local maker_pending = tonumber(redis.call('HGET', maker_quote_key, 'pending_available') or '0')

			local actual_deduct = math.min(maker_pending, maker_receive)
			if actual_deduct > 0 then
				redis.call('HINCRBYFLOAT', maker_quote_key, 'pending_available', -actual_deduct)
				redis.call('HINCRBYFLOAT', maker_quote_key, 'pending_total', -actual_deduct)
			end
			redis.call('HINCRBYFLOAT', maker_base_key, 'pending_frozen', base_amount)
			redis.call('HINCRBY', maker_quote_key, 'version', 1)
			redis.call('HINCRBY', maker_base_key, 'version', 1)
			redis.call('HSET', maker_quote_key, 'updated_at', now)
			redis.call('HSET', maker_base_key, 'updated_at', now)

			-- 原操作: Taker 扣 quote_frozen, 得 base_pending_available
			-- 回滚: Taker 扣 base_pending_available, 加 quote_pending_frozen
			local taker_receive = base_amount - taker_fee
			local taker_pending = tonumber(redis.call('HGET', taker_base_key, 'pending_available') or '0')

			local taker_actual_deduct = math.min(taker_pending, taker_receive)
			if taker_actual_deduct > 0 then
				redis.call('HINCRBYFLOAT', taker_base_key, 'pending_available', -taker_actual_deduct)
				redis.call('HINCRBYFLOAT', taker_base_key, 'pending_total', -taker_actual_deduct)
			end
			redis.call('HINCRBYFLOAT', taker_quote_key, 'pending_frozen', quote_amount)
			redis.call('HINCRBY', taker_base_key, 'version', 1)
			redis.call('HINCRBY', taker_quote_key, 'version', 1)
			redis.call('HSET', taker_base_key, 'updated_at', now)
			redis.call('HSET', taker_quote_key, 'updated_at', now)
		end

		-- 3. 标记已回滚 (7 天 TTL)
		redis.call('SETEX', rollback_key, 604800, '1')

		return {ok = 'success'}
	`)

	// luaDecrUserOpenOrders 减少用户活跃订单计数
	luaDecrUserOpenOrders = redis.NewScript(`
		local key = KEYS[1]
		local current = tonumber(redis.call('GET', key) or '0')
		if current > 0 then
			redis.call('DECR', key)
		end
		return current
	`)
)
