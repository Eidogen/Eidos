// Package service 提供风控服务的业务逻辑
//
// ========================================
// RiskService 风控服务对接说明
// ========================================
//
// ## 功能概述
// RiskService 提供交易前风控检查和提现风控检查，是所有交易和提现操作的前置守门员。
//
// ## 调用方 (gRPC Client)
// - eidos-api: 调用 CheckOrder/CheckWithdraw 进行前置风控检查
//
// ## 消息输出 (Kafka Producer)
// - Topic: risk-alerts
// - 消息类型: RiskAlertMessage
// - 触发条件: 检测到高风险行为、触发风控规则
//
// ## eidos-api 对接
// 1. 下单前调用 CheckOrder
//    - 时机: 签名验证通过后，发送到 matching 之前
//    - 处理: approved=false 时直接返回错误给用户
//
// 2. 提现前调用 CheckWithdraw
//    - 时机: 签名验证通过后，创建提现记录之前
//    - 处理: require_manual_review=true 时创建待审核记录
//
// ## eidos-trading 对接
// 1. 订单状态变更时同步到风控缓存
//    - 新订单创建: 调用 cache 更新挂单计数
//    - 订单完成/取消: 调用 cache 更新挂单计数和待结算金额
//
// ========================================
package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/config"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/repository"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/rules"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// RiskService 风控服务
type RiskService struct {
	// 仓储层
	ruleRepo       *repository.RiskRuleRepository
	eventRepo      *repository.RiskEventRepository
	blacklistRepo  *repository.BlacklistRepository
	auditRepo      *repository.AuditLogRepository
	withdrawRepo   *repository.WithdrawalReviewRepository

	// 缓存层
	blacklistCache *cache.BlacklistCache
	rateLimitCache *cache.RateLimitCache
	amountCache    *cache.AmountCache
	orderCache     *cache.OrderCache
	marketCache    *cache.MarketCache
	withdrawCache  *cache.WithdrawCache

	// 规则引擎
	engine *rules.Engine

	// 配置
	config *config.Config

	// Kafka 生产者 (通过回调设置)
	onRiskAlert func(ctx context.Context, alert *RiskAlertMessage) error

	// 服务降级
	degradationSvc *DegradationService
}

// RiskAlertMessage 风控告警消息
type RiskAlertMessage struct {
	AlertID     string            `json:"alert_id"`
	Wallet      string            `json:"wallet"`
	AlertType   string            `json:"alert_type"`
	Severity    string            `json:"severity"` // info, warning, critical
	RuleID      string            `json:"rule_id,omitempty"`
	Description string            `json:"description"`
	ActionTaken string            `json:"action_taken,omitempty"`
	Context     map[string]string `json:"context"`
	OrderID     string            `json:"order_id,omitempty"`
	WithdrawID  string            `json:"withdraw_id,omitempty"`
	CreatedAt   int64             `json:"created_at"`
}

// NewRiskService 创建风控服务
func NewRiskService(
	ruleRepo *repository.RiskRuleRepository,
	eventRepo *repository.RiskEventRepository,
	blacklistRepo *repository.BlacklistRepository,
	auditRepo *repository.AuditLogRepository,
	withdrawRepo *repository.WithdrawalReviewRepository,
	blacklistCache *cache.BlacklistCache,
	rateLimitCache *cache.RateLimitCache,
	amountCache *cache.AmountCache,
	orderCache *cache.OrderCache,
	marketCache *cache.MarketCache,
	withdrawCache *cache.WithdrawCache,
	cfg *config.Config,
) *RiskService {
	svc := &RiskService{
		ruleRepo:       ruleRepo,
		eventRepo:      eventRepo,
		blacklistRepo:  blacklistRepo,
		auditRepo:      auditRepo,
		withdrawRepo:   withdrawRepo,
		blacklistCache: blacklistCache,
		rateLimitCache: rateLimitCache,
		amountCache:    amountCache,
		orderCache:     orderCache,
		marketCache:    marketCache,
		withdrawCache:  withdrawCache,
		config:         cfg,
	}

	// 初始化规则引擎
	svc.initEngine()

	return svc
}

// SetOnRiskAlert 设置风控告警回调
func (s *RiskService) SetOnRiskAlert(fn func(ctx context.Context, alert *RiskAlertMessage) error) {
	s.onRiskAlert = fn
}

// SetDegradationService 设置降级服务
func (s *RiskService) SetDegradationService(svc *DegradationService) {
	s.degradationSvc = svc
}

// initEngine 初始化规则引擎
func (s *RiskService) initEngine() {
	s.engine = rules.NewEngine()

	// 注册检查器 (按优先级)
	// 1. 黑名单检查 (最高优先级)
	s.engine.RegisterChecker(
		rules.NewBlacklistChecker(s.blacklistCache),
		rules.PriorityHighest,
	)

	// 2. 频率限制检查
	rateLimitConfig := &rules.RateLimitConfig{
		OrdersPerSecond:  s.config.Risk.RateLimits.OrdersPerSecond,
		OrdersPerMinute:  s.config.Risk.RateLimits.OrdersPerMinute,
		CancelsPerMinute: s.config.Risk.RateLimits.CancelsPerMinute,
	}
	s.engine.RegisterChecker(
		rules.NewRateLimitChecker(s.rateLimitCache, rateLimitConfig),
		rules.PriorityHigh,
	)

	// 3. 金额限制检查
	amountConfig := &rules.AmountLimitConfig{
		SingleOrderMax:      s.config.Risk.Limits.GetSingleOrderLimit(),
		DailyWithdrawMax:    s.config.Risk.Limits.GetDailyWithdrawLimit(),
		SingleWithdrawMax:   decimal.NewFromInt(10000),
		PendingSettleMax:    s.config.Risk.Limits.GetPendingSettleLimit(),
		LargeTradeThreshold: s.config.Risk.Monitoring.GetLargeTradeThreshold(),
	}
	s.engine.RegisterChecker(
		rules.NewAmountChecker(s.amountCache, amountConfig),
		rules.PriorityNormal,
	)

	// 4. 自成交检查
	s.engine.RegisterChecker(
		rules.NewSelfTradeChecker(s.orderCache),
		rules.PriorityNormal,
	)

	// 5. 价格偏离检查
	priceConfig := &rules.PriceDeviationConfig{
		MaxDeviationPercent: decimal.NewFromFloat(0.1),
		WarningPercent:      decimal.NewFromFloat(0.05),
	}
	s.engine.RegisterChecker(
		rules.NewPriceChecker(s.marketCache, priceConfig),
		rules.PriorityNormal,
	)

	// 6. 提现安全检查
	withdrawConfig := &rules.WithdrawConfig{
		MinWithdrawAmount:    decimal.NewFromFloat(0.001),
		RequireWhitelistAddr: false,
		HighRiskThreshold:    decimal.NewFromInt(10000),
	}
	s.engine.RegisterChecker(
		rules.NewWithdrawChecker(s.withdrawCache, withdrawConfig),
		rules.PriorityNormal,
	)

	logger.Info("risk engine initialized",
		"checkers", s.engine.GetCheckerNames())
}

// CheckOrder 检查下单请求
func (s *RiskService) CheckOrder(ctx context.Context, req *CheckOrderRequest) (*CheckOrderResponse, error) {
	startTime := time.Now()

	// 检查服务降级状态
	if s.degradationSvc != nil && !s.degradationSvc.CanPerformRiskCheck() {
		approved, reason := s.degradationSvc.GetRiskCheckFallbackResult()
		return &CheckOrderResponse{
			Approved:     approved,
			RejectReason: reason,
			RejectCode:   "RISK_SERVICE_DEGRADED",
			Warnings:     []string{"风控服务处于降级模式"},
		}, nil
	}

	// 构建规则引擎请求
	engineReq := &rules.OrderCheckRequest{
		Wallet:    req.Wallet,
		Market:    req.Market,
		Side:      req.Side,
		OrderType: req.OrderType,
		Price:     req.Price,
		Amount:    req.Amount,
		Notional:  req.Price.Mul(req.Amount),
	}

	// 执行规则检查
	result := s.engine.CheckOrder(ctx, engineReq)

	// 记录审计日志
	duration := time.Since(startTime).Milliseconds()
	s.recordAuditLog(ctx, model.AuditActionCheckOrder, req.Wallet, req, result, int(duration))

	// 如果被拒绝，记录风控事件
	if !result.Passed {
		s.recordRiskEvent(ctx, &RiskEventParams{
			EventType:     model.RiskEventTypeOrderCheck,
			Wallet:        req.Wallet,
			Market:        req.Market,
			ReferenceID:   "",
			ReferenceType: "ORDER",
			Amount:        engineReq.Notional,
			RuleID:        result.RuleID,
			RuleName:      result.RuleName,
			Result:        model.RiskEventResultRejected,
			Reason:        result.Reason,
		})

		// 发送风控告警
		s.sendAlert(ctx, &RiskAlertMessage{
			AlertID:     uuid.New().String(),
			Wallet:      req.Wallet,
			AlertType:   "ORDER_REJECTED",
			Severity:    "warning",
			Description: result.Reason,
			Context: map[string]string{
				"market":    req.Market,
				"side":      req.Side,
				"rule_id":   result.RuleID,
				"rule_name": result.RuleName,
			},
			CreatedAt: time.Now().UnixMilli(),
		})
	}

	return &CheckOrderResponse{
		Approved:     result.Passed,
		RejectReason: result.Reason,
		RejectCode:   result.Code,
		Warnings:     result.Warnings,
	}, nil
}

// CheckWithdraw 检查提现请求
func (s *RiskService) CheckWithdraw(ctx context.Context, req *CheckWithdrawRequest) (*CheckWithdrawResponse, error) {
	startTime := time.Now()

	// 检查服务降级状态
	if s.degradationSvc != nil && !s.degradationSvc.CanPerformRiskCheck() {
		approved, reason := s.degradationSvc.GetRiskCheckFallbackResult()
		return &CheckWithdrawResponse{
			Approved:            approved,
			RejectReason:        reason,
			RejectCode:          "RISK_SERVICE_DEGRADED",
			RequireManualReview: !approved, // 降级时如果放行也需要人工审核
			RiskScore:           100,       // 高风险分数
		}, nil
	}

	// 构建规则引擎请求
	engineReq := &rules.WithdrawCheckRequest{
		Wallet:    req.Wallet,
		Token:     req.Token,
		Amount:    req.Amount,
		ToAddress: req.ToAddress,
	}

	// 执行规则检查
	result := s.engine.CheckWithdraw(ctx, engineReq)

	// 记录审计日志
	duration := time.Since(startTime).Milliseconds()
	s.recordAuditLog(ctx, model.AuditActionCheckWithdraw, req.Wallet, req, result, int(duration))

	// 判断是否需要人工审核
	requireManualReview := false
	if result.Passed && result.RiskScore >= 50 {
		requireManualReview = true

		// 创建提现审核记录
		s.createWithdrawReview(ctx, req, result)
	}

	// 如果被拒绝，记录风控事件
	if !result.Passed {
		s.recordRiskEvent(ctx, &RiskEventParams{
			EventType:     model.RiskEventTypeWithdrawCheck,
			Wallet:        req.Wallet,
			Token:         req.Token,
			ReferenceID:   req.WithdrawalID,
			ReferenceType: "WITHDRAW",
			Amount:        req.Amount,
			RuleID:        result.RuleID,
			RuleName:      result.RuleName,
			Result:        model.RiskEventResultRejected,
			Reason:        result.Reason,
		})

		// 发送风控告警
		s.sendAlert(ctx, &RiskAlertMessage{
			AlertID:     uuid.New().String(),
			Wallet:      req.Wallet,
			AlertType:   "WITHDRAW_REJECTED",
			Severity:    "warning",
			Description: result.Reason,
			Context: map[string]string{
				"token":      req.Token,
				"amount":     req.Amount.String(),
				"to_address": req.ToAddress,
				"rule_id":    result.RuleID,
			},
			CreatedAt: time.Now().UnixMilli(),
		})
	}

	return &CheckWithdrawResponse{
		Approved:            result.Passed,
		RejectReason:        result.Reason,
		RejectCode:          result.Code,
		RequireManualReview: requireManualReview,
		RiskScore:           result.RiskScore,
	}, nil
}

// CheckOrderRequest 下单检查请求
type CheckOrderRequest struct {
	Wallet    string
	Market    string
	Side      string
	OrderType string
	Price     decimal.Decimal
	Amount    decimal.Decimal
}

// CheckOrderResponse 下单检查响应
type CheckOrderResponse struct {
	Approved     bool
	RejectReason string
	RejectCode   string
	Warnings     []string
}

// CheckWithdrawRequest 提现检查请求
type CheckWithdrawRequest struct {
	WithdrawalID string
	Wallet       string
	Token        string
	Amount       decimal.Decimal
	ToAddress    string
}

// CheckWithdrawResponse 提现检查响应
type CheckWithdrawResponse struct {
	Approved            bool
	RejectReason        string
	RejectCode          string
	RequireManualReview bool
	ReviewReason        string
	RiskScore           int
}

// RiskEventParams 风控事件参数
type RiskEventParams struct {
	EventType     model.RiskEventType
	Wallet        string
	Market        string
	Token         string
	ReferenceID   string
	ReferenceType string
	Amount        decimal.Decimal
	RuleID        string
	RuleName      string
	Result        model.RiskEventResult
	Reason        string
}

// recordAuditLog 记录审计日志
func (s *RiskService) recordAuditLog(ctx context.Context, action model.AuditAction, wallet string, request interface{}, result *rules.CheckResult, durationMs int) {
	reqJSON, _ := json.Marshal(request)

	auditResult := model.AuditResultAllowed
	reason := ""
	if result != nil && !result.Passed {
		auditResult = model.AuditResultRejected
		reason = result.Reason
	}

	log := &model.AuditLog{
		Action:        action,
		WalletAddress: wallet,
		Request:       string(reqJSON),
		Result:        auditResult,
		Reason:        reason,
		DurationMs:    durationMs,
	}

	if err := s.auditRepo.Create(ctx, log); err != nil {
		logger.Error("failed to create audit log", "error", err)
	}
}

// recordRiskEvent 记录风控事件
func (s *RiskService) recordRiskEvent(ctx context.Context, params *RiskEventParams) {
	event := &model.RiskEvent{
		EventID:       uuid.New().String(),
		Type:          params.EventType,
		Wallet:        params.Wallet,
		Market:        params.Market,
		Token:         params.Token,
		ReferenceID:   params.ReferenceID,
		ReferenceType: params.ReferenceType,
		Amount:        params.Amount,
		RuleID:        params.RuleID,
		RuleName:      params.RuleName,
		Result:        params.Result,
		Reason:        params.Reason,
	}

	if err := s.eventRepo.Create(ctx, event); err != nil {
		logger.Error("failed to create risk event", "error", err)
	}
}

// createWithdrawReview 创建提现审核记录
func (s *RiskService) createWithdrawReview(ctx context.Context, req *CheckWithdrawRequest, result *rules.CheckResult) {
	riskFactors, _ := json.Marshal(result.Warnings)

	autoDecision := model.AutoDecisionManualReview
	if result.RiskScore < 30 {
		autoDecision = model.AutoDecisionAutoApprove
	} else if result.RiskScore >= 80 {
		autoDecision = model.AutoDecisionAutoReject
	}

	review := &model.WithdrawalReview{
		ReviewID:      uuid.New().String(),
		WithdrawalID:  req.WithdrawalID,
		WalletAddress: req.Wallet,
		Token:         req.Token,
		Amount:        req.Amount,
		ToAddress:     req.ToAddress,
		RiskScore:     result.RiskScore,
		RiskFactors:   string(riskFactors),
		AutoDecision:  autoDecision,
		Status:        model.WithdrawalReviewStatusPending,
		ExpiresAt:     time.Now().Add(24 * time.Hour).UnixMilli(),
	}

	if err := s.withdrawRepo.Create(ctx, review); err != nil {
		logger.Error("failed to create withdrawal review", "error", err)
	}
}

// sendAlert 发送风控告警
func (s *RiskService) sendAlert(ctx context.Context, alert *RiskAlertMessage) {
	if s.onRiskAlert != nil {
		if err := s.onRiskAlert(ctx, alert); err != nil {
			logger.Error("failed to send risk alert",
				"alert_id", alert.AlertID,
				"error", err)
		}
	}
}

// GetUserLimits 获取用户限额信息
func (s *RiskService) GetUserLimits(ctx context.Context, wallet string) (*rules.UserLimits, error) {
	amountChecker := rules.NewAmountChecker(s.amountCache, &rules.AmountLimitConfig{
		SingleOrderMax:   s.config.Risk.Limits.GetSingleOrderLimit(),
		DailyWithdrawMax: s.config.Risk.Limits.GetDailyWithdrawLimit(),
		PendingSettleMax: s.config.Risk.Limits.GetPendingSettleLimit(),
	})

	limits, err := amountChecker.GetUserLimits(ctx, wallet)
	if err != nil {
		return nil, err
	}

	return &rules.UserLimits{
		Wallet: wallet,
		Limits: limits,
	}, nil
}
