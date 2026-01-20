package model

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

// WithdrawStatus 提现状态
type WithdrawStatus int8

const (
	WithdrawStatusPending    WithdrawStatus = 0 // 待处理 (签名验证通过)
	WithdrawStatusProcessing WithdrawStatus = 1 // 处理中 (风控通过，准备上链)
	WithdrawStatusSubmitted  WithdrawStatus = 2 // 已提交 (链上交易已广播)
	WithdrawStatusConfirmed  WithdrawStatus = 3 // 已确认 (链上交易确认)
	WithdrawStatusFailed     WithdrawStatus = 4 // 失败 (链上交易失败)
	WithdrawStatusCancelled  WithdrawStatus = 5 // 已取消
	WithdrawStatusRejected   WithdrawStatus = 6 // 被拒绝 (风控拦截)
)

func (s WithdrawStatus) String() string {
	switch s {
	case WithdrawStatusPending:
		return "PENDING"
	case WithdrawStatusProcessing:
		return "PROCESSING"
	case WithdrawStatusSubmitted:
		return "SUBMITTED"
	case WithdrawStatusConfirmed:
		return "CONFIRMED"
	case WithdrawStatusFailed:
		return "FAILED"
	case WithdrawStatusCancelled:
		return "CANCELLED"
	case WithdrawStatusRejected:
		return "REJECTED"
	default:
		return "UNKNOWN"
	}
}

// IsTerminal 判断是否为终态
func (s WithdrawStatus) IsTerminal() bool {
	return s == WithdrawStatusConfirmed || s == WithdrawStatusFailed ||
		s == WithdrawStatusCancelled || s == WithdrawStatusRejected
}

// Withdrawal 提现记录
// 对应数据库表 withdrawals
// 幂等键: wallet + nonce
type Withdrawal struct {
	ID           int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	WithdrawID   string          `gorm:"type:varchar(64);uniqueIndex;not null" json:"withdraw_id"`            // 提现 ID
	Wallet       string          `gorm:"type:varchar(42);uniqueIndex:uk_wallet_nonce;not null" json:"wallet"` // 用户钱包
	Token        string          `gorm:"type:varchar(20);not null" json:"token"`                              // 代币
	Amount       decimal.Decimal `gorm:"type:decimal(36,18);not null" json:"amount"`                          // 提现金额
	ToAddress    string          `gorm:"type:varchar(42);not null" json:"to_address"`                         // 目标地址
	Nonce        uint64          `gorm:"type:bigint;uniqueIndex:uk_wallet_nonce;not null" json:"nonce"`       // 用户 Nonce
	Signature    []byte          `gorm:"type:bytea" json:"signature"`                                         // EIP-712 签名
	Status       WithdrawStatus  `gorm:"type:smallint;index;not null;default:0" json:"status"`                // 状态
	TxHash       string          `gorm:"type:varchar(66)" json:"tx_hash"`                                     // 链上交易哈希
	RejectReason string          `gorm:"type:varchar(255)" json:"reject_reason"`                              // 拒绝原因
	RefundedAt   int64           `gorm:"type:bigint" json:"refunded_at"`                                      // 退回时间 (失败时)
	SubmittedAt  int64           `gorm:"type:bigint" json:"submitted_at"`                                     // 提交时间
	ConfirmedAt  int64           `gorm:"type:bigint" json:"confirmed_at"`                                     // 确认时间
	CreatedAt    int64           `gorm:"type:bigint;not null;autoCreateTime:milli" json:"created_at"`
	UpdatedAt    int64           `gorm:"type:bigint;not null;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 返回表名
func (Withdrawal) TableName() string {
	return "trading_withdrawals"
}

// GetIdempotencyKey 获取幂等键
func (w *Withdrawal) GetIdempotencyKey() string {
	return fmt.Sprintf("%s:%d", w.Wallet, w.Nonce)
}

// ValidateToAddress 验证目标地址
// 检查: 格式、EIP-55 校验和、非零地址
func ValidateWithdrawAddress(address string) error {
	// 长度检查
	if len(address) != 42 {
		return fmt.Errorf("invalid address length: expected 42, got %d", len(address))
	}

	// 前缀检查
	if !strings.HasPrefix(address, "0x") {
		return fmt.Errorf("address must start with 0x")
	}

	// 解析地址
	addr := common.HexToAddress(address)

	// 零地址检查
	if addr == (common.Address{}) {
		return fmt.Errorf("cannot withdraw to zero address")
	}

	// EIP-55 Checksum 校验
	// 如果地址包含大写字母，必须是正确的 checksum 格式
	checksumAddr := addr.Hex()
	if address != strings.ToLower(address) && address != checksumAddr {
		return fmt.Errorf("invalid address checksum: expected %s", checksumAddr)
	}

	return nil
}
