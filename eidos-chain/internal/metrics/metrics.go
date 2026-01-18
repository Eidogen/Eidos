// Package metrics 提供 eidos-chain 服务的 Prometheus 监控指标
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "eidos_chain"

// 结算指标
var (
	// SettlementsTotal 结算总数
	SettlementsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "settlements_total",
			Help:      "结算总数",
		},
		[]string{"status"}, // pending, submitted, confirmed, failed, rolled_back
	)

	// SettlementDuration 结算耗时
	SettlementDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "settlement_duration_seconds",
			Help:      "结算耗时(秒)",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
		},
		[]string{"market"},
	)

	// SettlementBatchSize 结算批次大小
	SettlementBatchSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "settlement_batch_size",
			Help:      "结算批次包含的交易数量",
			Buckets:   []float64{1, 5, 10, 20, 50, 100, 200, 500},
		},
	)

	// PendingSettlementsGauge 待结算数量
	PendingSettlementsGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "pending_settlements_total",
			Help:      "当前待结算交易数量",
		},
	)
)

// 区块链交互指标
var (
	// BlockchainTxTotal 链上交易总数
	BlockchainTxTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "blockchain_tx_total",
			Help:      "链上交易总数",
		},
		[]string{"type", "status"}, // type: settlement/withdraw, status: success/failed/pending
	)

	// BlockchainTxDuration 链上交易耗时
	BlockchainTxDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "blockchain_tx_duration_seconds",
			Help:      "链上交易确认耗时(秒)",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"type"},
	)

	// BlockchainGasUsed Gas 使用量
	BlockchainGasUsed = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "blockchain_gas_used",
			Help:      "链上交易 Gas 使用量",
			Buckets:   []float64{21000, 50000, 100000, 200000, 500000, 1000000, 2000000},
		},
		[]string{"type"},
	)

	// BlockchainGasPrice Gas 价格
	BlockchainGasPrice = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "blockchain_gas_price_gwei",
			Help:      "当前 Gas 价格 (Gwei)",
		},
	)

	// BlockchainNonce 当前 Nonce
	BlockchainNonceGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "blockchain_nonce_current",
			Help:      "当前链上 Nonce",
		},
	)
)

// 区块索引指标
var (
	// BlocksIndexedTotal 已索引区块总数
	BlocksIndexedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "blocks_indexed_total",
			Help:      "已索引区块总数",
		},
	)

	// BlockIndexLatency 区块索引延迟
	BlockIndexLatency = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "block_index_latency_blocks",
			Help:      "区块索引延迟 (落后链上区块数)",
		},
	)

	// LatestIndexedBlock 最新索引区块高度
	LatestIndexedBlockGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "latest_indexed_block",
			Help:      "最新索引区块高度",
		},
	)

	// LatestChainBlock 链上最新区块高度
	LatestChainBlockGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "latest_chain_block",
			Help:      "链上最新区块高度",
		},
	)

	// EventsProcessedTotal 已处理事件总数
	EventsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "events_processed_total",
			Help:      "已处理链上事件总数",
		},
		[]string{"event_type"}, // deposit, withdraw, settlement, transfer
	)
)

// 充值指标
var (
	// DepositsTotal 充值总数
	DepositsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "deposits_total",
			Help:      "充值总数",
		},
		[]string{"token", "status"}, // status: detected, confirmed, credited, failed
	)

	// DepositAmount 充值金额
	DepositAmountTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "deposit_amount_total",
			Help:      "充值金额总计",
		},
		[]string{"token"},
	)

	// DepositConfirmationTime 充值确认时间
	DepositConfirmationTime = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "deposit_confirmation_seconds",
			Help:      "充值确认耗时(秒)",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800},
		},
		[]string{"token"},
	)

	// PendingDepositsGauge 待确认充值数量
	PendingDepositsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "pending_deposits_total",
			Help:      "待确认充值数量",
		},
		[]string{"token"},
	)
)

// 提现指标
var (
	// WithdrawalsTotal 提现总数
	WithdrawalsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "withdrawals_total",
			Help:      "提现总数",
		},
		[]string{"token", "status"}, // status: pending, submitted, confirmed, failed
	)

	// WithdrawalAmount 提现金额
	WithdrawalAmountTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "withdrawal_amount_total",
			Help:      "提现金额总计",
		},
		[]string{"token"},
	)

	// WithdrawalProcessingTime 提现处理时间
	WithdrawalProcessingTime = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "withdrawal_processing_seconds",
			Help:      "提现处理耗时(秒)",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"token"},
	)

	// PendingWithdrawalsGauge 待处理提现数量
	PendingWithdrawalsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "pending_withdrawals_total",
			Help:      "待处理提现数量",
		},
		[]string{"token"},
	)
)

// 重组检测指标
var (
	// ReorgsDetectedTotal 检测到的重组总数
	ReorgsDetectedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "reorgs_detected_total",
			Help:      "检测到的区块链重组总数",
		},
	)

	// ReorgDepth 重组深度
	ReorgDepth = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "reorg_depth_blocks",
			Help:      "重组深度 (回滚区块数)",
			Buckets:   []float64{1, 2, 3, 5, 10, 20, 50},
		},
	)

	// RolledBackSettlementsTotal 因重组回滚的结算总数
	RolledBackSettlementsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rolled_back_settlements_total",
			Help:      "因区块链重组回滚的结算总数",
		},
	)
)

// gRPC 服务指标
var (
	// GRPCRequestsTotal gRPC 请求总数
	GRPCRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "grpc_requests_total",
			Help:      "gRPC 请求总数",
		},
		[]string{"method", "code"},
	)

	// GRPCRequestDuration gRPC 请求耗时
	GRPCRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "grpc_request_duration_seconds",
			Help:      "gRPC 请求耗时(秒)",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		},
		[]string{"method"},
	)
)

// Kafka 指标
var (
	// KafkaMessagesConsumed Kafka 消费消息数
	KafkaMessagesConsumed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "kafka_messages_consumed_total",
			Help:      "Kafka 消费消息总数",
		},
		[]string{"topic"},
	)

	// KafkaMessagesProduced Kafka 生产消息数
	KafkaMessagesProduced = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "kafka_messages_produced_total",
			Help:      "Kafka 生产消息总数",
		},
		[]string{"topic"},
	)

	// KafkaConsumerLag Kafka 消费延迟
	KafkaConsumerLag = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "kafka_consumer_lag",
			Help:      "Kafka 消费延迟",
		},
		[]string{"topic", "partition"},
	)
)

// 数据库指标
var (
	// DBQueryDuration 数据库查询耗时
	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "db_query_duration_seconds",
			Help:      "数据库查询耗时(秒)",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		},
		[]string{"operation", "table"},
	)
)

// Helper functions

// RecordSettlement 记录结算
func RecordSettlement(status, market string, durationSeconds float64, batchSize int) {
	SettlementsTotal.WithLabelValues(status).Inc()
	if durationSeconds > 0 {
		SettlementDuration.WithLabelValues(market).Observe(durationSeconds)
	}
	if batchSize > 0 {
		SettlementBatchSize.Observe(float64(batchSize))
	}
}

// RecordBlockchainTx 记录链上交易
func RecordBlockchainTx(txType, status string, durationSeconds float64, gasUsed uint64) {
	BlockchainTxTotal.WithLabelValues(txType, status).Inc()
	if durationSeconds > 0 {
		BlockchainTxDuration.WithLabelValues(txType).Observe(durationSeconds)
	}
	if gasUsed > 0 {
		BlockchainGasUsed.WithLabelValues(txType).Observe(float64(gasUsed))
	}
}

// RecordBlockIndexed 记录区块索引
func RecordBlockIndexed(blockNumber uint64, chainHead uint64) {
	BlocksIndexedTotal.Inc()
	LatestIndexedBlockGauge.Set(float64(blockNumber))
	LatestChainBlockGauge.Set(float64(chainHead))
	BlockIndexLatency.Set(float64(chainHead - blockNumber))
}

// RecordEvent 记录链上事件
func RecordEvent(eventType string) {
	EventsProcessedTotal.WithLabelValues(eventType).Inc()
}

// RecordDeposit 记录充值
func RecordDeposit(token, status string, amount float64, confirmationSeconds float64) {
	DepositsTotal.WithLabelValues(token, status).Inc()
	if amount > 0 && status == "credited" {
		DepositAmountTotal.WithLabelValues(token).Add(amount)
	}
	if confirmationSeconds > 0 {
		DepositConfirmationTime.WithLabelValues(token).Observe(confirmationSeconds)
	}
}

// RecordWithdrawal 记录提现
func RecordWithdrawal(token, status string, amount float64, processingSeconds float64) {
	WithdrawalsTotal.WithLabelValues(token, status).Inc()
	if amount > 0 && status == "confirmed" {
		WithdrawalAmountTotal.WithLabelValues(token).Add(amount)
	}
	if processingSeconds > 0 {
		WithdrawalProcessingTime.WithLabelValues(token).Observe(processingSeconds)
	}
}

// RecordReorg 记录重组
func RecordReorg(depth int, settlementsRolledBack int) {
	ReorgsDetectedTotal.Inc()
	ReorgDepth.Observe(float64(depth))
	for i := 0; i < settlementsRolledBack; i++ {
		RolledBackSettlementsTotal.Inc()
	}
}

// RecordGRPCRequest 记录 gRPC 请求
func RecordGRPCRequest(method, code string, durationSeconds float64) {
	GRPCRequestsTotal.WithLabelValues(method, code).Inc()
	GRPCRequestDuration.WithLabelValues(method).Observe(durationSeconds)
}

// RecordKafkaMessage 记录 Kafka 消息
func RecordKafkaMessage(topic string, produced bool) {
	if produced {
		KafkaMessagesProduced.WithLabelValues(topic).Inc()
	} else {
		KafkaMessagesConsumed.WithLabelValues(topic).Inc()
	}
}

// UpdateGasPrice 更新 Gas 价格
func UpdateGasPrice(gasPriceGwei float64) {
	BlockchainGasPrice.Set(gasPriceGwei)
}

// UpdateNonce 更新 Nonce
func UpdateNonce(nonce uint64) {
	BlockchainNonceGauge.Set(float64(nonce))
}

// UpdatePendingSettlements 更新待结算数量
func UpdatePendingSettlements(count int) {
	PendingSettlementsGauge.Set(float64(count))
}

// UpdatePendingDeposits 更新待确认充值数量
func UpdatePendingDeposits(token string, count int) {
	PendingDepositsGauge.WithLabelValues(token).Set(float64(count))
}

// UpdatePendingWithdrawals 更新待处理提现数量
func UpdatePendingWithdrawals(token string, count int) {
	PendingWithdrawalsGauge.WithLabelValues(token).Set(float64(count))
}
