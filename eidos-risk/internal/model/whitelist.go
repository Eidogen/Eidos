// Package model defines the whitelist data models
package model

// WhitelistType represents the type of whitelist entry
type WhitelistType string

const (
	WhitelistTypeAddress    WhitelistType = "address"    // Withdrawal address whitelist
	WhitelistTypeVIP        WhitelistType = "vip"        // VIP user whitelist (higher limits)
	WhitelistTypeMarketMaker WhitelistType = "market_maker" // Market maker whitelist
	WhitelistTypeInternal   WhitelistType = "internal"   // Internal accounts
)

// WhitelistStatus represents the status of a whitelist entry
type WhitelistStatus string

const (
	WhitelistStatusActive    WhitelistStatus = "active"
	WhitelistStatusInactive  WhitelistStatus = "inactive"
	WhitelistStatusPending   WhitelistStatus = "pending"   // Pending approval
	WhitelistStatusExpired   WhitelistStatus = "expired"
)

// WhitelistEntry represents a whitelist entry
type WhitelistEntry struct {
	ID            int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	EntryID       string          `gorm:"column:entry_id;type:varchar(64);uniqueIndex;not null" json:"entry_id"`
	WalletAddress string          `gorm:"column:wallet_address;type:varchar(42);index;not null" json:"wallet_address"`
	TargetAddress string          `gorm:"column:target_address;type:varchar(42)" json:"target_address"` // For address whitelist
	ListType      WhitelistType   `gorm:"column:list_type;type:varchar(30);index;not null" json:"list_type"`
	Status        WhitelistStatus `gorm:"column:status;type:varchar(20);index;not null;default:'pending'" json:"status"`
	Label         string          `gorm:"column:label;type:varchar(100)" json:"label"` // User-friendly label
	Remark        string          `gorm:"column:remark;type:varchar(500)" json:"remark"`
	Metadata      string          `gorm:"column:metadata;type:jsonb" json:"metadata"` // Additional metadata
	EffectiveFrom int64           `gorm:"column:effective_from;type:bigint;not null" json:"effective_from"`
	EffectiveUntil int64          `gorm:"column:effective_until;type:bigint" json:"effective_until"` // 0 = permanent
	CreatedBy     string          `gorm:"column:created_by;type:varchar(42)" json:"created_by"`
	CreatedAt     int64           `gorm:"column:created_at;type:bigint;not null" json:"created_at"`
	UpdatedBy     string          `gorm:"column:updated_by;type:varchar(42)" json:"updated_by"`
	UpdatedAt     int64           `gorm:"column:updated_at;type:bigint;not null" json:"updated_at"`
	ApprovedBy    string          `gorm:"column:approved_by;type:varchar(42)" json:"approved_by"`
	ApprovedAt    int64           `gorm:"column:approved_at;type:bigint" json:"approved_at"`
}

// TableName returns the table name
func (WhitelistEntry) TableName() string {
	return "risk_whitelist"
}

// IsActive checks if the entry is active
func (w *WhitelistEntry) IsActive() bool {
	return w.Status == WhitelistStatusActive
}

// IsPermanent checks if the entry is permanent
func (w *WhitelistEntry) IsPermanent() bool {
	return w.EffectiveUntil == 0
}

// IsExpired checks if the entry has expired
func (w *WhitelistEntry) IsExpired(now int64) bool {
	if w.EffectiveUntil == 0 {
		return false
	}
	return now > w.EffectiveUntil
}

// IsEffective checks if the entry is within effective period
func (w *WhitelistEntry) IsEffective(now int64) bool {
	if now < w.EffectiveFrom {
		return false
	}
	if w.EffectiveUntil > 0 && now > w.EffectiveUntil {
		return false
	}
	return true
}

// IsValid checks if the entry is valid (active and within effective period)
func (w *WhitelistEntry) IsValid(now int64) bool {
	return w.IsActive() && w.IsEffective(now)
}

// WhitelistMetadata represents additional metadata for whitelist entries
type WhitelistMetadata struct {
	DailyWithdrawLimit  string            `json:"daily_withdraw_limit,omitempty"`  // Custom daily limit
	SingleOrderLimit    string            `json:"single_order_limit,omitempty"`    // Custom single order limit
	RateMultiplier      float64           `json:"rate_multiplier,omitempty"`       // Rate limit multiplier
	SkipChecks          []string          `json:"skip_checks,omitempty"`           // Checks to skip
	CustomParams        map[string]string `json:"custom_params,omitempty"`         // Custom parameters
	VerificationMethod  string            `json:"verification_method,omitempty"`   // How address was verified
	ExternalReference   string            `json:"external_reference,omitempty"`    // External system reference
}
