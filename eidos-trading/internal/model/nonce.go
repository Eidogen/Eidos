package model

import "fmt"

// NonceUsage Nonce 用途类型
type NonceUsage int8

const (
	NonceUsageOrder    NonceUsage = 1 // 订单
	NonceUsageCancel   NonceUsage = 2 // 取消订单
	NonceUsageWithdraw NonceUsage = 3 // 提现
)

func (u NonceUsage) String() string {
	switch u {
	case NonceUsageOrder:
		return "ORDER"
	case NonceUsageCancel:
		return "CANCEL"
	case NonceUsageWithdraw:
		return "WITHDRAW"
	default:
		return "UNKNOWN"
	}
}

// UsedNonce 已使用的 Nonce 记录
// 用于防止签名重放攻击
// 对应数据库表 used_nonces
type UsedNonce struct {
	ID        int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	Wallet    string     `gorm:"type:varchar(42);uniqueIndex:uk_wallet_usage_nonce;not null" json:"wallet"`
	Usage     NonceUsage `gorm:"type:smallint;uniqueIndex:uk_wallet_usage_nonce;not null" json:"usage"` // 用途
	Nonce     uint64     `gorm:"type:bigint;uniqueIndex:uk_wallet_usage_nonce;not null" json:"nonce"`
	OrderID   string     `gorm:"type:varchar(64)" json:"order_id"` // 关联订单 ID
	CreatedAt int64      `gorm:"type:bigint;not null;autoCreateTime:milli;index" json:"created_at"`
}

// TableName 返回表名
func (UsedNonce) TableName() string {
	return "used_nonces"
}

// GetRedisKey 获取 Redis 缓存键
func (n *UsedNonce) GetRedisKey() string {
	return fmt.Sprintf("nonce:%s:%d:%d", n.Wallet, n.Usage, n.Nonce)
}

// GetNonceRedisKey 生成 Nonce Redis 键
func GetNonceRedisKey(wallet string, usage NonceUsage, nonce uint64) string {
	return fmt.Sprintf("nonce:%s:%d:%d", wallet, usage, nonce)
}
