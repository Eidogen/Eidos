package model

import (
	"github.com/shopspring/decimal"
)

// SettlementStatus 结算状态
type SettlementStatus int8

const (
	SettlementStatusMatchedOffchain SettlementStatus = 0 // 链下成交 (撮合完成)
	SettlementStatusPending         SettlementStatus = 1 // 结算待处理 (已加入批次)
	SettlementStatusSubmitted       SettlementStatus = 2 // 结算已提交 (链上交易已广播)
	SettlementStatusSettledOnchain  SettlementStatus = 3 // 链上已结算 (交易确认)
	SettlementStatusFailed          SettlementStatus = 4 // 结算失败
	SettlementStatusRolledBack      SettlementStatus = 5 // 已回滚
)

func (s SettlementStatus) String() string {
	switch s {
	case SettlementStatusMatchedOffchain:
		return "MATCHED_OFFCHAIN"
	case SettlementStatusPending:
		return "SETTLEMENT_PENDING"
	case SettlementStatusSubmitted:
		return "SETTLEMENT_SUBMITTED"
	case SettlementStatusSettledOnchain:
		return "SETTLED_ONCHAIN"
	case SettlementStatusFailed:
		return "SETTLEMENT_FAILED"
	case SettlementStatusRolledBack:
		return "ROLLED_BACK"
	default:
		return "UNKNOWN"
	}
}

// Trade 成交记录
// 对应数据库表 trades
type Trade struct {
	ID               int64            `gorm:"primaryKey;autoIncrement" json:"id"`
	TradeID          string           `gorm:"type:varchar(64);uniqueIndex;not null" json:"trade_id"`           // 成交 ID
	Market           string           `gorm:"type:varchar(20);index;not null" json:"market"`                   // 交易对
	MakerOrderID     string           `gorm:"type:varchar(64);index;not null" json:"maker_order_id"`           // Maker 订单 ID
	TakerOrderID     string           `gorm:"type:varchar(64);index;not null" json:"taker_order_id"`           // Taker 订单 ID
	MakerWallet      string           `gorm:"type:varchar(42);index;not null" json:"maker_wallet"`             // Maker 钱包
	TakerWallet      string           `gorm:"type:varchar(42);index;not null" json:"taker_wallet"`             // Taker 钱包
	Price            decimal.Decimal  `gorm:"type:decimal(36,18);not null" json:"price"`                       // 成交价格
	Amount           decimal.Decimal  `gorm:"type:decimal(36,18);not null" json:"amount"`                      // 成交数量 (Base Token)
	QuoteAmount      decimal.Decimal  `gorm:"type:decimal(36,18);not null" json:"quote_amount"`                // 成交金额 (Quote Token)
	MakerFee         decimal.Decimal  `gorm:"type:decimal(36,18);default:0" json:"maker_fee"`                  // Maker 手续费
	TakerFee         decimal.Decimal  `gorm:"type:decimal(36,18);default:0" json:"taker_fee"`                  // Taker 手续费
	FeeToken         string           `gorm:"type:varchar(20)" json:"fee_token"`                               // 手续费 Token
	MakerSide        OrderSide        `gorm:"type:smallint;not null" json:"maker_side"`                        // Maker 方向 (买/卖)
	SettlementStatus SettlementStatus `gorm:"type:smallint;index;not null;default:0" json:"settlement_status"` // 结算状态
	BatchID          string           `gorm:"type:varchar(64);index" json:"batch_id"`                          // 结算批次 ID
	TxHash           string           `gorm:"type:varchar(66)" json:"tx_hash"`                                 // 链上交易哈希
	MatchedAt        int64            `gorm:"type:bigint;index;not null" json:"matched_at"`                    // 撮合时间 (毫秒)
	SettledAt        int64            `gorm:"type:bigint" json:"settled_at"`                                   // 结算确认时间 (毫秒)
	CreatedAt        int64            `gorm:"type:bigint;not null;autoCreateTime:milli" json:"created_at"`
	UpdatedAt        int64            `gorm:"type:bigint;not null;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 返回表名
func (Trade) TableName() string {
	return "trading_trades"
}

// TakerSide 返回 Taker 方向 (与 Maker 相反)
func (t *Trade) TakerSide() OrderSide {
	if t.MakerSide == OrderSideBuy {
		return OrderSideSell
	}
	return OrderSideBuy
}

// SettlementBatch 结算批次
// 对应数据库表 settlement_batches
type SettlementBatch struct {
	ID          int64            `gorm:"primaryKey;autoIncrement" json:"id"`
	BatchID     string           `gorm:"type:varchar(64);uniqueIndex;not null" json:"batch_id"` // 批次 ID
	TradeCount  int              `gorm:"type:int;not null" json:"trade_count"`                  // 成交笔数
	TotalAmount decimal.Decimal  `gorm:"type:decimal(36,18);not null" json:"total_amount"`      // 总金额
	Status      SettlementStatus `gorm:"type:smallint;index;not null" json:"status"`            // 批次状态
	TxHash      string           `gorm:"type:varchar(66)" json:"tx_hash"`                       // 链上交易哈希
	GasPrice    int64            `gorm:"type:bigint" json:"gas_price"`                          // Gas 价格
	GasUsed     int64            `gorm:"type:bigint" json:"gas_used"`                           // Gas 使用量
	RetryCount  int              `gorm:"type:int;default:0" json:"retry_count"`                 // 重试次数
	LastError   string           `gorm:"type:text" json:"last_error"`                           // 最后错误信息
	SubmittedAt int64            `gorm:"type:bigint" json:"submitted_at"`                       // 提交时间
	ConfirmedAt int64            `gorm:"type:bigint" json:"confirmed_at"`                       // 确认时间
	CreatedAt   int64            `gorm:"type:bigint;not null;autoCreateTime:milli" json:"created_at"`
	UpdatedAt   int64            `gorm:"type:bigint;not null;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 返回表名
func (SettlementBatch) TableName() string {
	return "trading_settlement_batches"
}
