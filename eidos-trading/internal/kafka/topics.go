// Package kafka 提供 Kafka 生产者和消费者
package kafka

// Kafka topic 名称
// 参考: docs/3-开发规范/00-协议总表.md
const (
	// 订单相关
	TopicOrders         = "orders"          // 新订单 (trading → matching)
	TopicCancelRequests = "cancel-requests" // 取消请求 (trading → matching)
	TopicOrderCancelled = "order-cancelled" // 订单已取消 (matching → trading)
	TopicOrderUpdates   = "order-updates"   // 订单状态更新 (trading → api/ws)

	// 成交相关
	TopicTradeResults = "trade-results" // 成交结果 (matching → trading)

	// 结算相关
	TopicSettlements         = "settlements"          // 结算请求 (trading → chain)
	TopicSettlementConfirmed = "settlement-confirmed" // 结算确认 (chain → trading)

	// 充提相关
	TopicDeposits            = "deposits"             // 充值事件 (chain → trading)
	TopicWithdrawals         = "withdrawals"          // 提现请求 (trading → chain)
	TopicWithdrawalConfirmed = "withdrawal-confirmed" // 提现确认 (chain → trading)

	// 余额相关
	TopicBalanceUpdates = "balance-updates" // 余额变更 (trading → api/ws)

	// 行情相关
	TopicOrderbookUpdates = "orderbook-updates" // 订单簿更新 (matching → market)
	TopicKline1m          = "kline-1m"          // 1分钟K线 (market)

	// 风控相关
	TopicRiskAlerts = "risk-alerts" // 风控告警 (risk)
	// TopicWithdrawalReviewResults 提现审核结果 (risk → trading)
	TopicWithdrawalReviewResults = "withdrawal-review-results"

	// 死信队列
	TopicDeadLetter = "dead-letter" // 处理失败的消息
)

// Message Kafka 消息结构
type Message struct {
	Topic     string
	Key       []byte
	Value     []byte
	Partition int32
	Offset    int64
	Timestamp int64
}
