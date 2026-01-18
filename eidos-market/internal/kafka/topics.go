package kafka

// Kafka Topic 定义
const (
	// TopicTradeResults 成交结果
	// 由 eidos-matching 发送，eidos-market 消费
	// TODO: 需要 eidos-matching 实现发送逻辑
	TopicTradeResults = "trade-results"

	// TopicOrderBookUpdates 订单簿增量更新
	// 由 eidos-matching 发送，eidos-market 消费
	// TODO: 需要 eidos-matching 实现发送逻辑
	TopicOrderBookUpdates = "orderbook-updates"

	// TopicKline1m 1 分钟 K 线
	// 由 eidos-market 发送，供外部消费
	TopicKline1m = "kline-1m"
)

// ConsumerGroup 消费者组
const (
	GroupMarket = "eidos-market"
)
