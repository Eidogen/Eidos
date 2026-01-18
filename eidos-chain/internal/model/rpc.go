package model

// RPCEndpoint RPC 节点信息
type RPCEndpoint struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	ChainID     int64  `gorm:"column:chain_id;type:int;not null" json:"chain_id"`
	EndpointURL string `gorm:"column:endpoint_url;type:varchar(255);not null" json:"endpoint_url"`
	IsHealthy   bool   `gorm:"column:is_healthy;type:boolean;index;not null;default:true" json:"is_healthy"`
	LatencyMs   int    `gorm:"column:latency_ms;type:int" json:"latency_ms"`
	LastBlock   int64  `gorm:"column:last_block;type:bigint" json:"last_block"`
	ErrorCount  int    `gorm:"column:error_count;type:int;not null;default:0" json:"error_count"`
	LastCheckAt int64  `gorm:"column:last_check_at;type:bigint;not null" json:"last_check_at"`
	CreatedAt   int64  `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
	UpdatedAt   int64  `gorm:"column:updated_at;type:bigint;not null" json:"updated_at"`
}

// TableName 返回表名
func (RPCEndpoint) TableName() string {
	return "eidos_chain_rpc_endpoints"
}
