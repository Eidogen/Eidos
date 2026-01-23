package model

// TokenStatus 代币状态
type TokenStatus int

const (
	TokenStatusActive   TokenStatus = 1 // 活跃
	TokenStatusDisabled TokenStatus = 2 // 禁用
)

// TokenConfig 代币配置
// 管理支持的代币及其参数
type TokenConfig struct {
	ID              int64       `gorm:"primaryKey;column:id" json:"id"`
	Symbol          string      `gorm:"column:symbol;size:20;uniqueIndex" json:"symbol"`                 // 代币符号 (如 USDC, ETH)
	Name            string      `gorm:"column:name;size:100" json:"name"`                                // 代币名称
	ContractAddress string      `gorm:"column:contract_address;size:42" json:"contract_address"`         // 合约地址 (原生代币为空)
	Decimals        int         `gorm:"column:decimals;default:18" json:"decimals"`                      // 小数位数
	ChainID         int64       `gorm:"column:chain_id;default:42161" json:"chain_id"`                   // 链 ID (默认 Arbitrum)
	Status          TokenStatus `gorm:"column:status;default:1" json:"status"`                           // 状态
	DepositEnabled  bool        `gorm:"column:deposit_enabled;default:true" json:"deposit_enabled"`      // 是否允许充值
	WithdrawEnabled bool        `gorm:"column:withdraw_enabled;default:true" json:"withdraw_enabled"`    // 是否允许提现
	MinDeposit      string      `gorm:"column:min_deposit;size:36;default:'0'" json:"min_deposit"`       // 最小充值金额
	MinWithdraw     string      `gorm:"column:min_withdraw;size:36;default:'0'" json:"min_withdraw"`     // 最小提现金额
	MaxWithdraw     string      `gorm:"column:max_withdraw;size:36;default:'0'" json:"max_withdraw"`     // 单笔最大提现金额
	WithdrawFee     string      `gorm:"column:withdraw_fee;size:36;default:'0'" json:"withdraw_fee"`     // 提现手续费
	Confirmations   int         `gorm:"column:confirmations;default:0" json:"confirmations"`             // 充值确认数
	DisplayOrder    int         `gorm:"column:display_order;default:0" json:"display_order"`             // 显示顺序
	IconURL         string      `gorm:"column:icon_url;size:255" json:"icon_url"`                        // 图标 URL
	Description     string      `gorm:"column:description;size:500" json:"description"`                  // 描述
	CreatedBy       int64       `gorm:"column:created_by" json:"created_by"`
	CreatedAt       int64       `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedBy       int64       `gorm:"column:updated_by" json:"updated_by"`
	UpdatedAt       int64       `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
}

// TableName 表名
func (TokenConfig) TableName() string {
	return "admin_token_configs"
}

// IsNativeToken 是否为原生代币 (如 ETH)
func (t *TokenConfig) IsNativeToken() bool {
	return t.ContractAddress == "" || t.ContractAddress == "0x0000000000000000000000000000000000000000"
}

// IsActive 是否活跃
func (t *TokenConfig) IsActive() bool {
	return t.Status == TokenStatusActive
}

// CanDeposit 是否可充值
func (t *TokenConfig) CanDeposit() bool {
	return t.IsActive() && t.DepositEnabled
}

// CanWithdraw 是否可提现
func (t *TokenConfig) CanWithdraw() bool {
	return t.IsActive() && t.WithdrawEnabled
}
