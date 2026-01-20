package model

// BlockCheckpoint 区块检查点
type BlockCheckpoint struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	ChainID     int64  `gorm:"column:chain_id;type:int;uniqueIndex;not null" json:"chain_id"`
	BlockNumber int64  `gorm:"column:block_number;type:bigint;not null" json:"block_number"`
	BlockHash   string `gorm:"column:block_hash;type:varchar(66);not null" json:"block_hash"`
	ProcessedAt int64  `gorm:"column:processed_at;type:bigint;not null" json:"processed_at"`
	CreatedAt   int64  `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
	UpdatedAt   int64  `gorm:"column:updated_at;type:bigint;not null" json:"updated_at"`
}

// TableName 返回表名
func (BlockCheckpoint) TableName() string {
	return "chain_block_checkpoints"
}

// ChainEventType 链上事件类型
type ChainEventType string

const (
	ChainEventTypeDeposit    ChainEventType = "Deposit"
	ChainEventTypeWithdraw   ChainEventType = "Withdraw"
	ChainEventTypeSettlement ChainEventType = "Settlement"
)

// ChainEvent 链上事件
type ChainEvent struct {
	ID          int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	ChainID     int64          `gorm:"column:chain_id;type:int;not null" json:"chain_id"`
	BlockNumber int64          `gorm:"column:block_number;type:bigint;index;not null" json:"block_number"`
	TxHash      string         `gorm:"column:tx_hash;type:varchar(66);index;not null" json:"tx_hash"`
	LogIndex    int            `gorm:"column:log_index;type:int;not null" json:"log_index"`
	EventType   ChainEventType `gorm:"column:event_type;type:varchar(50);index;not null" json:"event_type"`
	EventData   string         `gorm:"column:event_data;type:jsonb;not null" json:"event_data"` // JSON
	Processed   bool           `gorm:"column:processed;type:boolean;index;not null;default:false" json:"processed"`
	CreatedAt   int64          `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
	UpdatedAt   int64          `gorm:"column:updated_at;type:bigint;not null" json:"updated_at"`
}

// TableName 返回表名
func (ChainEvent) TableName() string {
	return "chain_events"
}
