// Package rules provides database-based rule source
package rules

import (
	"context"
	"encoding/json"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/repository"
)

// DBRuleSource loads rules from database
type DBRuleSource struct {
	repo        *repository.RiskRuleRepository
	pollPeriod  time.Duration
	lastVersion int
}

// NewDBRuleSource creates a new database rule source
func NewDBRuleSource(repo *repository.RiskRuleRepository, pollPeriod time.Duration) *DBRuleSource {
	if pollPeriod == 0 {
		pollPeriod = 30 * time.Second
	}

	return &DBRuleSource{
		repo:       repo,
		pollPeriod: pollPeriod,
	}
}

// Name returns the source name
func (s *DBRuleSource) Name() string {
	return "database"
}

// Load loads all enabled rules from database
func (s *DBRuleSource) Load(ctx context.Context) ([]*RuleConfig, error) {
	rules, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}

	configs := make([]*RuleConfig, 0, len(rules))
	for _, rule := range rules {
		config := s.convertToConfig(rule)
		if config != nil {
			configs = append(configs, config)
		}
	}

	logger.Debug("rules loaded from database",
		"count", len(configs))

	return configs, nil
}

// Watch watches for rule changes in database
func (s *DBRuleSource) Watch(ctx context.Context, updates chan<- []*RuleConfig) error {
	ticker := time.NewTicker(s.pollPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			rules, err := s.Load(ctx)
			if err != nil {
				logger.Error("failed to poll rules from database", "error", err)
				continue
			}

			// Check if there are changes
			currentVersion := s.calculateVersion(rules)
			if currentVersion != s.lastVersion {
				s.lastVersion = currentVersion
				select {
				case updates <- rules:
				default:
					logger.Warn("rule update channel full, skipping")
				}
			}
		}
	}
}

// convertToConfig converts a model.RiskRule to RuleConfig
func (s *DBRuleSource) convertToConfig(rule *model.RiskRule) *RuleConfig {
	config := &RuleConfig{
		RuleID:      rule.RuleID,
		Name:        rule.Name,
		Type:        rule.Type,
		Scope:       rule.Scope,
		ScopeValue:  rule.ScopeValue,
		Priority:    rule.Priority,
		Enabled:     rule.Status == model.RiskRuleStatusActive,
		Action:      rule.Action,
		EffectiveAt: rule.EffectiveAt,
		ExpireAt:    rule.ExpireAt,
		Params:      make(map[string]interface{}),
		Conditions:  make([]RuleCondition, 0),
	}

	// Parse rule parameters based on type
	switch rule.Type {
	case model.RiskRuleTypeAmountLimit:
		config.Params["threshold"] = rule.Threshold.String()
	case model.RiskRuleTypeRateLimit:
		config.Params["max_count"] = rule.MaxCount
		config.Params["period"] = rule.Period
	case model.RiskRuleTypeWithdrawLimit:
		config.Params["threshold"] = rule.Threshold.String()
		config.Params["period"] = rule.Period
	}

	return config
}

// calculateVersion calculates a simple version hash from rules
func (s *DBRuleSource) calculateVersion(rules []*RuleConfig) int {
	version := 0
	for _, rule := range rules {
		data, _ := json.Marshal(rule)
		for _, b := range data {
			version = (version * 31) + int(b)
		}
	}
	return version
}
