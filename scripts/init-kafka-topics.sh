#!/bin/bash
#
# Eidos Trading System - Kafka Topics Initialization Script
# Creates all required Kafka topics for the trading system
#

set -e

KAFKA_BOOTSTRAP_SERVER="${KAFKA_BOOTSTRAP_SERVER:-kafka:9092}"
REPLICATION_FACTOR="${REPLICATION_FACTOR:-1}"

echo "Waiting for Kafka to be ready..."
sleep 30

# Function to create topic if it doesn't exist
create_topic() {
    local topic=$1
    local partitions=$2
    local retention_hours=${3:-168}  # Default 7 days

    echo "Creating topic: $topic (partitions: $partitions, retention: ${retention_hours}h)"

    /opt/kafka/bin/kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP_SERVER" \
        --create \
        --if-not-exists \
        --topic "$topic" \
        --partitions "$partitions" \
        --replication-factor "$REPLICATION_FACTOR" \
        --config retention.ms=$((retention_hours * 3600000)) \
        || true
}

echo "=============================================="
echo "  Creating Kafka Topics for Eidos"
echo "=============================================="
echo ""

# 1. 订单流相关 Topic (Order Flow)
echo "Creating Order Flow Topics..."
create_topic "orders" 16                    # 用户提交的新订单，高频主题
create_topic "cancel-requests" 8            # 用户提交的取消订单请求
create_topic "order-accepted" 16            # 撮合引擎确认接收并放入订单簿的订单
create_topic "order-cancelled" 8            # 撮合引擎确认已取消的订单
create_topic "order-rejected" 8             # 订单校验失败、风控拒绝或撤单失败的消息
create_topic "order-updates" 16             # 订单状态变更的全量更新消息（用于 API 推送）

# 2. 交易/撮合结果相关 Topic (Trade Results)
echo "Creating Trade Topics..."
create_topic "trade-results" 16             # 撮合引擎产出的成交结果，包含成交价与成交量

# 3. 行情数据相关 Topic (Market Data)
echo "Creating Market Data Topics..."
create_topic "orderbook-updates" 16         # 实时订单簿（L2/L3）增量更新数据
create_topic "market-stats" 4               # 24小时行情统计数据（最高、最低、成交额等）

# 4. 账户资金相关 Topic (Account Balance)
echo "Creating Balance Topics..."
create_topic "balance-updates" 8            # 用户余额变动通知（充提、手续费、成交导致的变动）

# 5. 充值与提现相关 Topic (Deposit & Withdrawal)
echo "Creating Deposit/Withdrawal Topics..."
create_topic "deposits" 4                   # 监听到链上充值事件的消息
create_topic "deposit-confirmed" 4          # 充值入账成功的确认消息
create_topic "withdrawals" 4                # 用户发起的提现原始请求
create_topic "withdrawal-submitted" 4       # 提现交易已提交至区块链后的消息
create_topic "withdrawal-confirmed" 4       # 区块链确认提现成功的消息
create_topic "withdrawal-status" 4          # 提现生命周期中的状态变化详情

# 6. 结算相关 Topic (Settlement)
echo "Creating Settlement Topics..."
create_topic "settlements" 8                # 待提交到链上的成交结算批次数据
create_topic "settlement-submitted" 4       # 结算批次已提交上链的消息
create_topic "settlement-confirmed" 4       # 区块链确认结算收据的消息
create_topic "settlement-failed" 4          # 结算失败需人工介入或重试的消息

# 7. 风控相关 Topic (Risk Control)
echo "Creating Risk Topics..."
create_topic "risk-alerts" 4                # 触发风控阈值后的告警推送
create_topic "risk-checks" 8                # 实时下单过程中的异步风控检查请求

# 8. 通知与系统消息相关 Topic (Notifications)
echo "Creating Notification Topics..."
create_topic "notifications" 8              # 发送给用户的业务通知（App推送/邮件等）

# 9. 管理员指令相关 Topic (Admin Commands)
echo "Creating Admin Topics..."
create_topic "admin-commands" 2             # 管理后台下发的系统指令（手动撮合、维护模式等）
create_topic "audit-logs" 4                 # 核心操作的审计日志流

echo ""
echo "=============================================="
echo "  Topic Creation Complete"
echo "=============================================="

# List all topics
echo ""
echo "Current topics:"
/opt/kafka/bin/kafka-topics.sh --bootstrap-server "$KAFKA_BOOTSTRAP_SERVER" --list

echo ""
echo "Done!"
