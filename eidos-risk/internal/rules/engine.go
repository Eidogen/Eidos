package rules

import (
	"context"
	"sort"
	"sync"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"go.uber.org/zap"
)

// Engine 规则引擎
type Engine struct {
	mu       sync.RWMutex
	checkers []checkerWithPriority
}

type checkerWithPriority struct {
	checker  RuleChecker
	priority RulePriority
}

// NewEngine 创建规则引擎
func NewEngine() *Engine {
	return &Engine{
		checkers: make([]checkerWithPriority, 0),
	}
}

// RegisterChecker 注册检查器
func (e *Engine) RegisterChecker(checker RuleChecker, priority RulePriority) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.checkers = append(e.checkers, checkerWithPriority{
		checker:  checker,
		priority: priority,
	})

	// 按优先级排序 (数字越小优先级越高)
	sort.Slice(e.checkers, func(i, j int) bool {
		return e.checkers[i].priority < e.checkers[j].priority
	})

	logger.Info("rule checker registered",
		zap.String("name", checker.Name()),
		zap.Int("priority", int(priority)))
}

// CheckOrder 执行下单检查
func (e *Engine) CheckOrder(ctx context.Context, req *OrderCheckRequest) *CheckResult {
	e.mu.RLock()
	checkers := make([]checkerWithPriority, len(e.checkers))
	copy(checkers, e.checkers)
	e.mu.RUnlock()

	var warnings []string

	for _, c := range checkers {
		result := c.checker.CheckOrder(ctx, req)
		if result == nil {
			continue
		}

		// 收集警告
		if len(result.Warnings) > 0 {
			warnings = append(warnings, result.Warnings...)
		}

		// 如果检查未通过，立即返回
		if !result.Passed {
			logger.Warn("order check rejected",
				zap.String("checker", c.checker.Name()),
				zap.String("wallet", req.Wallet),
				zap.String("market", req.Market),
				zap.String("rule_id", result.RuleID),
				zap.String("reason", result.Reason))
			return result
		}
	}

	// 所有检查通过
	if len(warnings) > 0 {
		return NewPassedWithWarnings(warnings)
	}
	return NewPassedResult()
}

// CheckWithdraw 执行提现检查
func (e *Engine) CheckWithdraw(ctx context.Context, req *WithdrawCheckRequest) *CheckResult {
	e.mu.RLock()
	checkers := make([]checkerWithPriority, len(e.checkers))
	copy(checkers, e.checkers)
	e.mu.RUnlock()

	var warnings []string
	totalRiskScore := 0
	checkerCount := 0

	for _, c := range checkers {
		result := c.checker.CheckWithdraw(ctx, req)
		if result == nil {
			continue
		}

		// 累计风险分值
		if result.RiskScore > 0 {
			totalRiskScore += result.RiskScore
			checkerCount++
		}

		// 收集警告
		if len(result.Warnings) > 0 {
			warnings = append(warnings, result.Warnings...)
		}

		// 如果检查未通过，立即返回
		if !result.Passed {
			logger.Warn("withdraw check rejected",
				zap.String("checker", c.checker.Name()),
				zap.String("wallet", req.Wallet),
				zap.String("token", req.Token),
				zap.String("rule_id", result.RuleID),
				zap.String("reason", result.Reason))
			return result
		}
	}

	// 所有检查通过
	result := NewPassedResult()
	if len(warnings) > 0 {
		result.Warnings = warnings
	}

	// 计算平均风险分值
	if checkerCount > 0 {
		result.RiskScore = totalRiskScore / checkerCount
	}

	return result
}

// GetCheckerNames 获取所有检查器名称
func (e *Engine) GetCheckerNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	names := make([]string, len(e.checkers))
	for i, c := range e.checkers {
		names[i] = c.checker.Name()
	}
	return names
}
