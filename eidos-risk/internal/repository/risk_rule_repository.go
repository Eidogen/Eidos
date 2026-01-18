package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
)

var (
	ErrRuleNotFound      = errors.New("risk rule not found")
	ErrRuleDuplicate     = errors.New("risk rule already exists")
	ErrRuleVersionExists = errors.New("rule version already exists")
)

// RiskRuleRepository 风控规则仓储
type RiskRuleRepository struct {
	db *gorm.DB
}

// NewRiskRuleRepository 创建风控规则仓储
func NewRiskRuleRepository(db *gorm.DB) *RiskRuleRepository {
	return &RiskRuleRepository{db: db}
}

// Create 创建规则
func (r *RiskRuleRepository) Create(ctx context.Context, rule *model.RiskRule) error {
	now := time.Now().UnixMilli()
	rule.CreatedAt = now
	rule.UpdatedAt = now

	result := r.db.WithContext(ctx).Create(rule)
	if result.Error != nil {
		if isDuplicateKeyError(result.Error) {
			return ErrRuleDuplicate
		}
		return result.Error
	}
	return nil
}

// Update 更新规则
func (r *RiskRuleRepository) Update(ctx context.Context, rule *model.RiskRule) error {
	rule.UpdatedAt = time.Now().UnixMilli()

	result := r.db.WithContext(ctx).
		Model(&model.RiskRule{}).
		Where("rule_id = ?", rule.RuleID).
		Updates(map[string]interface{}{
			"name":         rule.Name,
			"description":  rule.Description,
			"threshold":    rule.Threshold,
			"period":       rule.Period,
			"max_count":    rule.MaxCount,
			"action":       rule.Action,
			"priority":     rule.Priority,
			"status":       rule.Status,
			"effective_at": rule.EffectiveAt,
			"expire_at":    rule.ExpireAt,
			"updated_by":   rule.UpdatedBy,
			"updated_at":   rule.UpdatedAt,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRuleNotFound
	}
	return nil
}

// GetByRuleID 根据规则ID获取
func (r *RiskRuleRepository) GetByRuleID(ctx context.Context, ruleID string) (*model.RiskRule, error) {
	var rule model.RiskRule
	err := r.db.WithContext(ctx).
		Where("rule_id = ?", ruleID).
		First(&rule).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRuleNotFound
		}
		return nil, err
	}
	return &rule, nil
}

// ListByType 根据类型查询规则列表
func (r *RiskRuleRepository) ListByType(ctx context.Context, ruleType string) ([]*model.RiskRule, error) {
	var rules []*model.RiskRule
	err := r.db.WithContext(ctx).
		Where("rule_type = ?", ruleType).
		Order("id ASC").
		Find(&rules).Error

	if err != nil {
		return nil, err
	}
	return rules, nil
}

// ListByMarket 根据市场查询规则列表
func (r *RiskRuleRepository) ListByMarket(ctx context.Context, market string) ([]*model.RiskRule, error) {
	var rules []*model.RiskRule
	err := r.db.WithContext(ctx).
		Where("scope_value = ? OR scope = ?", market, model.RiskRuleScopeGlobal).
		Where("status = ?", model.RiskRuleStatusActive).
		Order("priority ASC").
		Find(&rules).Error

	if err != nil {
		return nil, err
	}
	return rules, nil
}

// ListEnabled 查询所有启用的规则
func (r *RiskRuleRepository) ListEnabled(ctx context.Context) ([]*model.RiskRule, error) {
	var rules []*model.RiskRule
	now := time.Now().UnixMilli()
	err := r.db.WithContext(ctx).
		Where("status = ?", model.RiskRuleStatusActive).
		Where("effective_at <= ? OR effective_at = 0", now).
		Where("expire_at > ? OR expire_at = 0", now).
		Order("priority ASC").
		Find(&rules).Error

	if err != nil {
		return nil, err
	}
	return rules, nil
}

// ListAll 查询所有规则
func (r *RiskRuleRepository) ListAll(ctx context.Context, pagination *Pagination) ([]*model.RiskRule, int64, error) {
	var rules []*model.RiskRule
	var total int64

	query := r.db.WithContext(ctx).Model(&model.RiskRule{})

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	err := query.
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Order("id ASC").
		Find(&rules).Error

	if err != nil {
		return nil, 0, err
	}
	return rules, total, nil
}

// SetEnabled 设置规则启用状态
func (r *RiskRuleRepository) SetEnabled(ctx context.Context, ruleID string, enabled bool, updatedBy string) error {
	status := model.RiskRuleStatusInactive
	if enabled {
		status = model.RiskRuleStatusActive
	}

	result := r.db.WithContext(ctx).
		Model(&model.RiskRule{}).
		Where("rule_id = ?", ruleID).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_by": updatedBy,
			"updated_at": time.Now().UnixMilli(),
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRuleNotFound
	}
	return nil
}

// Delete 删除规则
func (r *RiskRuleRepository) Delete(ctx context.Context, ruleID string) error {
	result := r.db.WithContext(ctx).
		Where("rule_id = ?", ruleID).
		Delete(&model.RiskRule{})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRuleNotFound
	}
	return nil
}

// CreateVersion 创建规则版本记录
func (r *RiskRuleRepository) CreateVersion(ctx context.Context, version *model.RiskRuleVersion) error {
	result := r.db.WithContext(ctx).Create(version)
	if result.Error != nil {
		if isDuplicateKeyError(result.Error) {
			return ErrRuleVersionExists
		}
		return result.Error
	}
	return nil
}

// GetLatestVersion 获取规则最新版本号
func (r *RiskRuleRepository) GetLatestVersion(ctx context.Context, ruleID string) (int, error) {
	var version model.RiskRuleVersion
	err := r.db.WithContext(ctx).
		Where("rule_id = ?", ruleID).
		Order("version DESC").
		First(&version).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return version.Version, nil
}

// ListVersions 查询规则版本历史
func (r *RiskRuleRepository) ListVersions(ctx context.Context, ruleID string) ([]*model.RiskRuleVersion, error) {
	var versions []*model.RiskRuleVersion
	err := r.db.WithContext(ctx).
		Where("rule_id = ?", ruleID).
		Order("version DESC").
		Find(&versions).Error

	if err != nil {
		return nil, err
	}
	return versions, nil
}
