package model

import (
	"encoding/json"
)

// OutboxStatus 消息状态
type OutboxStatus string

const (
	OutboxStatusPending    OutboxStatus = "pending"    // 待发送
	OutboxStatusProcessing OutboxStatus = "processing" // 处理中 (已被某实例认领)
	OutboxStatusSent       OutboxStatus = "sent"       // 已发送
	OutboxStatusFailed     OutboxStatus = "failed"     // 发送失败
)

// OutboxMessage 本地消息表记录
type OutboxMessage struct {
	ID            int64        `gorm:"primaryKey;autoIncrement" json:"id"`
	MessageID     string       `gorm:"type:varchar(64);uniqueIndex;not null" json:"message_id"`             // 全局唯一 ID
	Topic         string       `gorm:"type:varchar(100);not null" json:"topic"`                             // Kafka topic
	PartitionKey  string       `gorm:"type:varchar(100);not null" json:"partition_key"`                     // 分区键
	Payload       []byte       `gorm:"type:jsonb;not null" json:"payload"`                                  // 消息内容
	AggregateType string       `gorm:"type:varchar(50);not null;index:idx_aggregate" json:"aggregate_type"` // order, cancel, withdraw
	AggregateID   string       `gorm:"type:varchar(64);not null;index:idx_aggregate" json:"aggregate_id"`   // 关联业务 ID
	Status        OutboxStatus `gorm:"type:varchar(20);not null;default:'pending';index:idx_status_created" json:"status"`
	RetryCount    int          `gorm:"type:int;not null;default:0" json:"retry_count"`
	MaxRetries    int          `gorm:"type:int;not null;default:5" json:"max_retries"`
	LastError     string       `gorm:"type:varchar(500)" json:"last_error"`
	CreatedAt     int64        `gorm:"type:bigint;not null;index:idx_status_created" json:"created_at"`
	UpdatedAt     int64        `gorm:"type:bigint" json:"updated_at"`
	SentAt        int64        `gorm:"type:bigint" json:"sent_at"`
}

// TableName 返回表名
func (OutboxMessage) TableName() string {
	return "trading_outbox_messages"
}

// SetPayload 设置消息内容
func (m *OutboxMessage) SetPayload(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	m.Payload = data
	return nil
}

// GetPayload 获取消息内容
func (m *OutboxMessage) GetPayload(v interface{}) error {
	return json.Unmarshal(m.Payload, v)
}

// AggregateType 常量
const (
	AggregateTypeOrder    = "order"
	AggregateTypeCancel   = "cancel"
	AggregateTypeWithdraw = "withdraw"
	AggregateTypeBalance  = "balance"
	AggregateTypeTrade    = "trade"
)

// Topic 常量 (用于 DB 操作类型)
const (
	TopicBalanceUpdate  = "balance_update"
	TopicBalanceLog     = "balance_log"
	TopicOrderUpdate    = "order_update"
	TopicTradeCreate    = "trade_create"
	TopicWithdrawUpdate = "withdraw_update"
)

// BalanceUpdatePayload 余额更新载荷
type BalanceUpdatePayload struct {
	Wallet           string `json:"wallet"`
	Token            string `json:"token"`
	SettledAvailable string `json:"settled_available"`
	SettledFrozen    string `json:"settled_frozen"`
	PendingAvailable string `json:"pending_available"`
	PendingFrozen    string `json:"pending_frozen"`
	PendingTotal     string `json:"pending_total"`
	Version          int64  `json:"version"`
}

// BalanceLogPayload 余额流水载荷
type BalanceLogPayload struct {
	Wallet        string `json:"wallet"`
	Token         string `json:"token"`
	Type          int8   `json:"type"`
	Amount        string `json:"amount"`
	BalanceBefore string `json:"balance_before"`
	BalanceAfter  string `json:"balance_after"`
	OrderID       string `json:"order_id,omitempty"`
	TradeID       string `json:"trade_id,omitempty"`
	TxHash        string `json:"tx_hash,omitempty"`
	Remark        string `json:"remark,omitempty"`
}
