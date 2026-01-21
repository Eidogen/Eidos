// Package rules provides dynamic rule loading and hot reload functionality
package rules

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
	"github.com/shopspring/decimal"
)

// RuleConfig represents a rule configuration loaded from database or Nacos
type RuleConfig struct {
	RuleID      string                 `json:"rule_id"`
	Name        string                 `json:"name"`
	Type        model.RiskRuleType     `json:"type"`
	Scope       model.RiskRuleScope    `json:"scope"`
	ScopeValue  string                 `json:"scope_value"`
	Priority    int                    `json:"priority"`
	Enabled     bool                   `json:"enabled"`
	Params      map[string]interface{} `json:"params"`
	Conditions  []RuleCondition        `json:"conditions"`
	Action      model.RiskAction       `json:"action"`
	EffectiveAt int64                  `json:"effective_at"`
	ExpireAt    int64                  `json:"expire_at"`
}

// RuleCondition represents a condition for rule evaluation
type RuleCondition struct {
	Field    string      `json:"field"`    // Field to check: amount, price, frequency, etc.
	Operator string      `json:"operator"` // gt, lt, gte, lte, eq, ne, in, not_in
	Value    interface{} `json:"value"`    // Value to compare
	LogicOp  string      `json:"logic_op"` // AND, OR (for chaining conditions)
}

// RuleSource represents a source for loading rules
type RuleSource interface {
	// Load loads all enabled rules from the source
	Load(ctx context.Context) ([]*RuleConfig, error)
	// Watch watches for rule changes and sends updates to the channel
	Watch(ctx context.Context, updates chan<- []*RuleConfig) error
	// Name returns the source name
	Name() string
}

// DynamicRuleLoader handles dynamic loading and hot reload of rules
type DynamicRuleLoader struct {
	mu sync.RWMutex

	sources      []RuleSource
	rules        map[string]*RuleConfig // rule_id -> config
	rulesByType  map[model.RiskRuleType][]*RuleConfig
	rulesByScope map[string][]*RuleConfig // scope_value -> configs

	engine       *Engine
	updateChan   chan []*RuleConfig
	stopChan     chan struct{}
	reloadPeriod time.Duration

	// Callbacks for rule changes
	onRuleChange func(ctx context.Context, changeType string, rules []*RuleConfig)
}

// NewDynamicRuleLoader creates a new dynamic rule loader
func NewDynamicRuleLoader(engine *Engine, reloadPeriod time.Duration) *DynamicRuleLoader {
	if reloadPeriod == 0 {
		reloadPeriod = 30 * time.Second
	}

	return &DynamicRuleLoader{
		sources:      make([]RuleSource, 0),
		rules:        make(map[string]*RuleConfig),
		rulesByType:  make(map[model.RiskRuleType][]*RuleConfig),
		rulesByScope: make(map[string][]*RuleConfig),
		engine:       engine,
		updateChan:   make(chan []*RuleConfig, 100),
		stopChan:     make(chan struct{}),
		reloadPeriod: reloadPeriod,
	}
}

// AddSource adds a rule source
func (l *DynamicRuleLoader) AddSource(source RuleSource) {
	l.sources = append(l.sources, source)
	logger.Info("rule source added", "source", source.Name())
}

// SetOnRuleChange sets the callback for rule changes
func (l *DynamicRuleLoader) SetOnRuleChange(fn func(ctx context.Context, changeType string, rules []*RuleConfig)) {
	l.onRuleChange = fn
}

// Start starts the dynamic rule loader
func (l *DynamicRuleLoader) Start(ctx context.Context) error {
	// Initial load from all sources
	if err := l.loadAll(ctx); err != nil {
		return err
	}

	// Start watching for changes from all sources
	for _, source := range l.sources {
		go func(s RuleSource) {
			if err := s.Watch(ctx, l.updateChan); err != nil {
				logger.Error("rule source watch error",
					"source", s.Name(),
					"error", err)
			}
		}(source)
	}

	// Start the update handler
	go l.handleUpdates(ctx)

	// Start periodic reload
	go l.periodicReload(ctx)

	logger.Info("dynamic rule loader started",
		"reload_period", l.reloadPeriod,
		"sources", len(l.sources))

	return nil
}

// Stop stops the dynamic rule loader
func (l *DynamicRuleLoader) Stop() {
	close(l.stopChan)
}

// loadAll loads rules from all sources
func (l *DynamicRuleLoader) loadAll(ctx context.Context) error {
	allRules := make([]*RuleConfig, 0)

	for _, source := range l.sources {
		rules, err := source.Load(ctx)
		if err != nil {
			logger.Error("failed to load rules from source",
				"source", source.Name(),
				"error", err)
			continue
		}
		allRules = append(allRules, rules...)
		logger.Info("rules loaded from source",
			"source", source.Name(),
			"count", len(rules))
	}

	l.applyRules(ctx, allRules)
	return nil
}

// applyRules applies loaded rules
func (l *DynamicRuleLoader) applyRules(ctx context.Context, rules []*RuleConfig) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Track changes
	var added, updated, removed []*RuleConfig
	newRules := make(map[string]*RuleConfig)

	// Process new rules
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		// Check if rule is within effective period
		now := time.Now().UnixMilli()
		if rule.EffectiveAt > 0 && now < rule.EffectiveAt {
			continue
		}
		if rule.ExpireAt > 0 && now > rule.ExpireAt {
			continue
		}

		newRules[rule.RuleID] = rule

		if existing, ok := l.rules[rule.RuleID]; ok {
			if !rulesEqual(existing, rule) {
				updated = append(updated, rule)
			}
		} else {
			added = append(added, rule)
		}
	}

	// Find removed rules
	for ruleID, rule := range l.rules {
		if _, ok := newRules[ruleID]; !ok {
			removed = append(removed, rule)
		}
	}

	// Update internal state
	l.rules = newRules
	l.rebuildIndexes()

	// Log changes
	if len(added) > 0 || len(updated) > 0 || len(removed) > 0 {
		logger.Info("rules updated",
			"added", len(added),
			"updated", len(updated),
			"removed", len(removed),
			"total", len(l.rules))

		// Notify callback
		if l.onRuleChange != nil {
			if len(added) > 0 {
				l.onRuleChange(ctx, "ADDED", added)
			}
			if len(updated) > 0 {
				l.onRuleChange(ctx, "UPDATED", updated)
			}
			if len(removed) > 0 {
				l.onRuleChange(ctx, "REMOVED", removed)
			}
		}
	}
}

// rebuildIndexes rebuilds the rule indexes
func (l *DynamicRuleLoader) rebuildIndexes() {
	l.rulesByType = make(map[model.RiskRuleType][]*RuleConfig)
	l.rulesByScope = make(map[string][]*RuleConfig)

	for _, rule := range l.rules {
		l.rulesByType[rule.Type] = append(l.rulesByType[rule.Type], rule)

		if rule.ScopeValue != "" {
			l.rulesByScope[rule.ScopeValue] = append(l.rulesByScope[rule.ScopeValue], rule)
		}
	}

	// Sort by priority within each type
	for ruleType := range l.rulesByType {
		sortRulesByPriority(l.rulesByType[ruleType])
	}
}

// handleUpdates handles rule updates from sources
func (l *DynamicRuleLoader) handleUpdates(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.stopChan:
			return
		case rules := <-l.updateChan:
			l.applyRules(ctx, rules)
		}
	}
}

// periodicReload periodically reloads rules
func (l *DynamicRuleLoader) periodicReload(ctx context.Context) {
	ticker := time.NewTicker(l.reloadPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-l.stopChan:
			return
		case <-ticker.C:
			if err := l.loadAll(ctx); err != nil {
				logger.Error("periodic rule reload failed", "error", err)
			}
		}
	}
}

// GetRule returns a rule by ID
func (l *DynamicRuleLoader) GetRule(ruleID string) *RuleConfig {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.rules[ruleID]
}

// GetRulesByType returns rules by type
func (l *DynamicRuleLoader) GetRulesByType(ruleType model.RiskRuleType) []*RuleConfig {
	l.mu.RLock()
	defer l.mu.RUnlock()

	rules := l.rulesByType[ruleType]
	result := make([]*RuleConfig, len(rules))
	copy(result, rules)
	return result
}

// GetRulesForMarket returns rules applicable to a specific market
func (l *DynamicRuleLoader) GetRulesForMarket(market string) []*RuleConfig {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*RuleConfig, 0)

	// Add global rules
	for _, rule := range l.rules {
		if rule.Scope == model.RiskRuleScopeGlobal {
			result = append(result, rule)
		}
	}

	// Add market-specific rules
	if marketRules, ok := l.rulesByScope[market]; ok {
		result = append(result, marketRules...)
	}

	sortRulesByPriority(result)
	return result
}

// GetRulesForUser returns rules applicable to a specific user
func (l *DynamicRuleLoader) GetRulesForUser(wallet string) []*RuleConfig {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*RuleConfig, 0)

	// Add global rules
	for _, rule := range l.rules {
		if rule.Scope == model.RiskRuleScopeGlobal {
			result = append(result, rule)
		}
	}

	// Add user-specific rules
	if userRules, ok := l.rulesByScope[wallet]; ok {
		result = append(result, userRules...)
	}

	sortRulesByPriority(result)
	return result
}

// EvaluateConditions evaluates rule conditions against a context
func (l *DynamicRuleLoader) EvaluateConditions(conditions []RuleCondition, ctx map[string]interface{}) bool {
	if len(conditions) == 0 {
		return true
	}

	result := true
	for i, cond := range conditions {
		condResult := l.evaluateCondition(cond, ctx)

		if i == 0 {
			result = condResult
		} else {
			switch cond.LogicOp {
			case "OR":
				result = result || condResult
			default: // AND
				result = result && condResult
			}
		}
	}

	return result
}

// evaluateCondition evaluates a single condition
func (l *DynamicRuleLoader) evaluateCondition(cond RuleCondition, ctx map[string]interface{}) bool {
	fieldValue, ok := ctx[cond.Field]
	if !ok {
		return false
	}

	// Convert values for comparison
	switch cond.Operator {
	case "gt", "lt", "gte", "lte":
		return l.compareNumeric(fieldValue, cond.Value, cond.Operator)
	case "eq":
		return l.compareEqual(fieldValue, cond.Value)
	case "ne":
		return !l.compareEqual(fieldValue, cond.Value)
	case "in":
		return l.compareIn(fieldValue, cond.Value)
	case "not_in":
		return !l.compareIn(fieldValue, cond.Value)
	case "contains":
		return l.compareContains(fieldValue, cond.Value)
	case "starts_with":
		return l.compareStartsWith(fieldValue, cond.Value)
	default:
		return false
	}
}

// compareNumeric compares numeric values
func (l *DynamicRuleLoader) compareNumeric(fieldValue, condValue interface{}, operator string) bool {
	fv := toDecimal(fieldValue)
	cv := toDecimal(condValue)

	switch operator {
	case "gt":
		return fv.GreaterThan(cv)
	case "lt":
		return fv.LessThan(cv)
	case "gte":
		return fv.GreaterThanOrEqual(cv)
	case "lte":
		return fv.LessThanOrEqual(cv)
	default:
		return false
	}
}

// compareEqual compares for equality
func (l *DynamicRuleLoader) compareEqual(fieldValue, condValue interface{}) bool {
	// Try numeric comparison first
	fv := toDecimal(fieldValue)
	cv := toDecimal(condValue)
	if !fv.IsZero() || !cv.IsZero() {
		return fv.Equal(cv)
	}

	// String comparison
	return toString(fieldValue) == toString(condValue)
}

// compareIn checks if value is in a list
func (l *DynamicRuleLoader) compareIn(fieldValue, condValue interface{}) bool {
	fvStr := toString(fieldValue)

	switch cv := condValue.(type) {
	case []interface{}:
		for _, v := range cv {
			if toString(v) == fvStr {
				return true
			}
		}
	case []string:
		for _, v := range cv {
			if v == fvStr {
				return true
			}
		}
	}

	return false
}

// compareContains checks if field contains value
func (l *DynamicRuleLoader) compareContains(fieldValue, condValue interface{}) bool {
	fvStr := toString(fieldValue)
	cvStr := toString(condValue)
	return len(fvStr) > 0 && len(cvStr) > 0 && containsString(fvStr, cvStr)
}

// compareStartsWith checks if field starts with value
func (l *DynamicRuleLoader) compareStartsWith(fieldValue, condValue interface{}) bool {
	fvStr := toString(fieldValue)
	cvStr := toString(condValue)
	return len(fvStr) >= len(cvStr) && fvStr[:len(cvStr)] == cvStr
}

// Helper functions

func rulesEqual(a, b *RuleConfig) bool {
	if a == nil || b == nil {
		return a == b
	}

	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}

func sortRulesByPriority(rules []*RuleConfig) {
	for i := 0; i < len(rules)-1; i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[i].Priority > rules[j].Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}
}

func toDecimal(v interface{}) decimal.Decimal {
	switch val := v.(type) {
	case decimal.Decimal:
		return val
	case float64:
		return decimal.NewFromFloat(val)
	case int64:
		return decimal.NewFromInt(val)
	case int:
		return decimal.NewFromInt(int64(val))
	case string:
		d, _ := decimal.NewFromString(val)
		return d
	default:
		return decimal.Zero
	}
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case decimal.Decimal:
		return val.String()
	default:
		data, _ := json.Marshal(v)
		return string(data)
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
