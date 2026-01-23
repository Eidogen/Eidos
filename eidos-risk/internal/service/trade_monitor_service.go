package service

import (
	"context"
	"sync"
	"time"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/cache"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/model"
	"github.com/eidos-exchange/eidos/eidos-risk/internal/repository"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// TradeMonitorService 交易监控服务 (后置风控)
// 用于检测交易后的异常行为：
// - 对敲交易 (Wash Trade)
// - 价格异常波动
// - 大单异常
// - 高频交易
type TradeMonitorService struct {
	eventRepo  *repository.RiskEventRepository
	orderCache *cache.OrderCache
	marketCache *cache.MarketCache

	// 价格历史缓存 (market -> []PricePoint)
	priceHistory   map[string][]PricePoint
	priceHistoryMu sync.RWMutex

	// 交易统计缓存 (market -> TradeStats)
	tradeStats   map[string]*TradeStats
	tradeStatsMu sync.RWMutex

	// 用户交易计数 (wallet -> TradeCount)
	userTradeCount   map[string]*UserTradeCount
	userTradeCountMu sync.RWMutex

	// Kafka 告警回调
	onRiskAlert func(ctx context.Context, alert *RiskAlertMessage) error

	// 配置
	config *TradeMonitorConfig
}

// TradeMonitorConfig 交易监控配置
type TradeMonitorConfig struct {
	// 价格异常检测
	PriceDeviationWarning  float64       // 价格波动告警阈值 (百分比)
	PriceDeviationCritical float64       // 价格波动严重阈值 (百分比)
	PriceCheckWindow       time.Duration // 价格检查窗口

	// 对敲交易检测
	WashTradeWindow    time.Duration // 对敲检测窗口
	WashTradeThreshold int           // 同一账户自成交次数阈值

	// 大单检测
	LargeOrderMultiplier float64 // 大单阈值 (相对日均值的倍数)

	// 高频交易检测
	HighFreqWindow    time.Duration // 高频检测窗口
	HighFreqThreshold int           // 高频订单数量阈值
}

// PricePoint 价格点
type PricePoint struct {
	Price     decimal.Decimal
	Timestamp time.Time
}

// TradeStats 交易统计
type TradeStats struct {
	DailyVolume    decimal.Decimal
	DailyTradeCount int64
	AvgOrderSize   decimal.Decimal
	LastUpdated    time.Time
}

// UserTradeCount 用户交易计数
type UserTradeCount struct {
	TradesInWindow int
	LastTradeTime  time.Time
	SelfTrades     int // 自成交计数
}

// TradeEvent 交易事件 (后置风控使用)
type TradeEvent struct {
	TradeID     string
	Market      string
	MakerWallet string
	TakerWallet string
	Price       decimal.Decimal
	Amount      decimal.Decimal
	Timestamp   time.Time
}

// NewTradeMonitorService 创建交易监控服务
func NewTradeMonitorService(
	eventRepo *repository.RiskEventRepository,
	orderCache *cache.OrderCache,
	marketCache *cache.MarketCache,
) *TradeMonitorService {
	return &TradeMonitorService{
		eventRepo:      eventRepo,
		orderCache:     orderCache,
		marketCache:    marketCache,
		priceHistory:   make(map[string][]PricePoint),
		tradeStats:     make(map[string]*TradeStats),
		userTradeCount: make(map[string]*UserTradeCount),
		config:         DefaultTradeMonitorConfig(),
	}
}

// DefaultTradeMonitorConfig 默认配置
func DefaultTradeMonitorConfig() *TradeMonitorConfig {
	return &TradeMonitorConfig{
		PriceDeviationWarning:  5.0,  // 5% 价格波动告警
		PriceDeviationCritical: 10.0, // 10% 价格波动严重
		PriceCheckWindow:       5 * time.Minute,
		WashTradeWindow:        1 * time.Hour,
		WashTradeThreshold:     50, // 1小时内50次自成交
		LargeOrderMultiplier:   10.0, // 10倍于日均值
		HighFreqWindow:         1 * time.Minute,
		HighFreqThreshold:      1000, // 1分钟1000笔订单
	}
}

// SetAlertCallback 设置告警回调
func (s *TradeMonitorService) SetAlertCallback(callback func(ctx context.Context, alert *RiskAlertMessage) error) {
	s.onRiskAlert = callback
}

// SetConfig 设置配置
func (s *TradeMonitorService) SetConfig(config *TradeMonitorConfig) {
	s.config = config
}

// TradeCallback 返回 Kafka 消费者使用的回调函数
func (s *TradeMonitorService) TradeCallback() func(ctx context.Context, tradeID, market, makerWallet, takerWallet string, price, amount decimal.Decimal, timestamp time.Time) {
	return func(ctx context.Context, tradeID, market, makerWallet, takerWallet string, price, amount decimal.Decimal, timestamp time.Time) {
		trade := &TradeEvent{
			TradeID:     tradeID,
			Market:      market,
			MakerWallet: makerWallet,
			TakerWallet: takerWallet,
			Price:       price,
			Amount:      amount,
			Timestamp:   timestamp,
		}
		s.ProcessTrade(ctx, trade)
	}
}

// ProcessTrade 处理交易事件 (后置风控入口)
func (s *TradeMonitorService) ProcessTrade(ctx context.Context, trade *TradeEvent) {
	// 1. 更新价格历史
	s.updatePriceHistory(trade.Market, trade.Price)

	// 2. 更新交易统计
	s.updateTradeStats(trade.Market, trade.Amount)

	// 3. 检测对敲交易
	s.checkWashTrade(ctx, trade)

	// 4. 检测价格异常
	s.checkPriceAnomaly(ctx, trade)

	// 5. 检测大单异常
	s.checkLargeOrder(ctx, trade)

	// 6. 检测高频交易
	s.checkHighFrequency(ctx, trade)
}

// updatePriceHistory 更新价格历史
func (s *TradeMonitorService) updatePriceHistory(market string, price decimal.Decimal) {
	s.priceHistoryMu.Lock()
	defer s.priceHistoryMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-s.config.PriceCheckWindow)

	// 获取历史记录
	history := s.priceHistory[market]

	// 清理过期记录
	var newHistory []PricePoint
	for _, p := range history {
		if p.Timestamp.After(cutoff) {
			newHistory = append(newHistory, p)
		}
	}

	// 添加新价格点
	newHistory = append(newHistory, PricePoint{
		Price:     price,
		Timestamp: now,
	})

	s.priceHistory[market] = newHistory
}

// updateTradeStats 更新交易统计
func (s *TradeMonitorService) updateTradeStats(market string, amount decimal.Decimal) {
	s.tradeStatsMu.Lock()
	defer s.tradeStatsMu.Unlock()

	now := time.Now()
	stats, exists := s.tradeStats[market]
	if !exists {
		stats = &TradeStats{}
		s.tradeStats[market] = stats
	}

	// 每日重置
	if stats.LastUpdated.Day() != now.Day() {
		stats.DailyVolume = decimal.Zero
		stats.DailyTradeCount = 0
		stats.AvgOrderSize = decimal.Zero
	}

	stats.DailyVolume = stats.DailyVolume.Add(amount)
	stats.DailyTradeCount++
	if stats.DailyTradeCount > 0 {
		stats.AvgOrderSize = stats.DailyVolume.Div(decimal.NewFromInt(stats.DailyTradeCount))
	}
	stats.LastUpdated = now
}

// checkWashTrade 检测对敲交易
func (s *TradeMonitorService) checkWashTrade(ctx context.Context, trade *TradeEvent) {
	// 对敲交易: Maker 和 Taker 是同一个人
	if trade.MakerWallet != trade.TakerWallet {
		return
	}

	s.userTradeCountMu.Lock()
	defer s.userTradeCountMu.Unlock()

	now := time.Now()
	wallet := trade.MakerWallet

	count, exists := s.userTradeCount[wallet]
	if !exists {
		count = &UserTradeCount{}
		s.userTradeCount[wallet] = count
	}

	// 窗口外则重置
	if now.Sub(count.LastTradeTime) > s.config.WashTradeWindow {
		count.SelfTrades = 0
	}

	count.SelfTrades++
	count.LastTradeTime = now

	// 检测是否超过阈值
	if count.SelfTrades >= s.config.WashTradeThreshold {
		s.recordEvent(ctx, &RiskEventParams{
			EventType:     model.RiskEventTypeTradeMonitor,
			Wallet:        wallet,
			Market:        trade.Market,
			ReferenceID:   trade.TradeID,
			ReferenceType: "TRADE",
			Amount:        trade.Price.Mul(trade.Amount),
			RuleID:        "WASH_TRADE_DETECTION",
			RuleName:      "对敲交易检测",
			Result:        model.RiskEventResultAlert,
			Reason:        "检测到对敲交易: 1小时内自成交超过阈值",
		})

		s.sendAlert(ctx, &RiskAlertMessage{
			AlertID:     uuid.New().String(),
			Wallet:      wallet,
			AlertType:   "WASH_TRADE_DETECTED",
			Severity:    "critical",
			Description: "检测到对敲交易: 同一账户1小时内自成交次数过多",
			Context: map[string]string{
				"market":      trade.Market,
				"self_trades": string(rune(count.SelfTrades)),
				"trade_id":    trade.TradeID,
			},
			CreatedAt: now.UnixMilli(),
		})

		logger.Warn("wash trade detected",
			"wallet", wallet,
			"market", trade.Market,
			"self_trades", count.SelfTrades)
	}
}

// checkPriceAnomaly 检测价格异常
func (s *TradeMonitorService) checkPriceAnomaly(ctx context.Context, trade *TradeEvent) {
	s.priceHistoryMu.RLock()
	defer s.priceHistoryMu.RUnlock()

	history := s.priceHistory[trade.Market]
	if len(history) < 2 {
		return
	}

	// 获取窗口内最早价格
	oldestPrice := history[0].Price
	currentPrice := trade.Price

	// 计算价格变化百分比
	if oldestPrice.IsZero() {
		return
	}

	priceChange := currentPrice.Sub(oldestPrice).Div(oldestPrice).Mul(decimal.NewFromInt(100))
	priceChangeAbs := priceChange.Abs()
	changeFloat, _ := priceChangeAbs.Float64()

	now := time.Now()

	// 严重价格异常
	if changeFloat >= s.config.PriceDeviationCritical {
		s.recordEvent(ctx, &RiskEventParams{
			EventType:     model.RiskEventTypeTradeMonitor,
			Wallet:        trade.TakerWallet,
			Market:        trade.Market,
			ReferenceID:   trade.TradeID,
			ReferenceType: "TRADE",
			Amount:        trade.Price.Mul(trade.Amount),
			RuleID:        "PRICE_ANOMALY_CRITICAL",
			RuleName:      "价格异常检测",
			Result:        model.RiskEventResultAlert,
			Reason:        "价格在5分钟内变化超过10%",
		})

		s.sendAlert(ctx, &RiskAlertMessage{
			AlertID:     uuid.New().String(),
			Wallet:      trade.TakerWallet,
			AlertType:   "PRICE_ANOMALY_CRITICAL",
			Severity:    "critical",
			Description: "价格异常波动: 5分钟内变化超过10%",
			Context: map[string]string{
				"market":       trade.Market,
				"old_price":    oldestPrice.String(),
				"new_price":    currentPrice.String(),
				"change_pct":   priceChange.String(),
			},
			CreatedAt: now.UnixMilli(),
		})

		logger.Warn("critical price anomaly detected",
			"market", trade.Market,
			"change_pct", changeFloat)
		return
	}

	// 告警级别价格异常
	if changeFloat >= s.config.PriceDeviationWarning {
		s.recordEvent(ctx, &RiskEventParams{
			EventType:     model.RiskEventTypeTradeMonitor,
			Wallet:        trade.TakerWallet,
			Market:        trade.Market,
			ReferenceID:   trade.TradeID,
			ReferenceType: "TRADE",
			Amount:        trade.Price.Mul(trade.Amount),
			RuleID:        "PRICE_ANOMALY_WARNING",
			RuleName:      "价格异常检测",
			Result:        model.RiskEventResultAlert,
			Reason:        "价格在5分钟内变化超过5%",
		})

		s.sendAlert(ctx, &RiskAlertMessage{
			AlertID:     uuid.New().String(),
			Wallet:      trade.TakerWallet,
			AlertType:   "PRICE_ANOMALY_WARNING",
			Severity:    "warning",
			Description: "价格异常波动: 5分钟内变化超过5%",
			Context: map[string]string{
				"market":       trade.Market,
				"old_price":    oldestPrice.String(),
				"new_price":    currentPrice.String(),
				"change_pct":   priceChange.String(),
			},
			CreatedAt: now.UnixMilli(),
		})
	}
}

// checkLargeOrder 检测大单异常
func (s *TradeMonitorService) checkLargeOrder(ctx context.Context, trade *TradeEvent) {
	s.tradeStatsMu.RLock()
	stats := s.tradeStats[trade.Market]
	s.tradeStatsMu.RUnlock()

	if stats == nil || stats.AvgOrderSize.IsZero() {
		return
	}

	orderSize := trade.Amount
	threshold := stats.AvgOrderSize.Mul(decimal.NewFromFloat(s.config.LargeOrderMultiplier))

	if orderSize.GreaterThan(threshold) {
		now := time.Now()

		s.recordEvent(ctx, &RiskEventParams{
			EventType:     model.RiskEventTypeLargeTrade,
			Wallet:        trade.TakerWallet,
			Market:        trade.Market,
			ReferenceID:   trade.TradeID,
			ReferenceType: "TRADE",
			Amount:        trade.Price.Mul(trade.Amount),
			RuleID:        "LARGE_ORDER_DETECTION",
			RuleName:      "大单异常检测",
			Result:        model.RiskEventResultAlert,
			Reason:        "单笔订单超过日均值10倍",
		})

		s.sendAlert(ctx, &RiskAlertMessage{
			AlertID:     uuid.New().String(),
			Wallet:      trade.TakerWallet,
			AlertType:   "LARGE_ORDER_DETECTED",
			Severity:    "warning",
			Description: "大单异常: 单笔交易金额超过日均值10倍",
			Context: map[string]string{
				"market":       trade.Market,
				"order_size":   orderSize.String(),
				"avg_size":     stats.AvgOrderSize.String(),
				"multiplier":   decimal.NewFromFloat(s.config.LargeOrderMultiplier).String(),
			},
			CreatedAt: now.UnixMilli(),
		})

		logger.Warn("large order detected",
			"market", trade.Market,
			"wallet", trade.TakerWallet,
			"order_size", orderSize.String(),
			"avg_size", stats.AvgOrderSize.String())
	}
}

// checkHighFrequency 检测高频交易
func (s *TradeMonitorService) checkHighFrequency(ctx context.Context, trade *TradeEvent) {
	s.userTradeCountMu.Lock()
	defer s.userTradeCountMu.Unlock()

	now := time.Now()

	for _, wallet := range []string{trade.MakerWallet, trade.TakerWallet} {
		count, exists := s.userTradeCount[wallet]
		if !exists {
			count = &UserTradeCount{}
			s.userTradeCount[wallet] = count
		}

		// 窗口外则重置
		if now.Sub(count.LastTradeTime) > s.config.HighFreqWindow {
			count.TradesInWindow = 0
		}

		count.TradesInWindow++
		count.LastTradeTime = now

		// 检测高频交易
		if count.TradesInWindow >= s.config.HighFreqThreshold {
			s.recordEvent(ctx, &RiskEventParams{
				EventType:     model.RiskEventTypeTradeMonitor,
				Wallet:        wallet,
				Market:        trade.Market,
				ReferenceID:   trade.TradeID,
				ReferenceType: "TRADE",
				Amount:        trade.Price.Mul(trade.Amount),
				RuleID:        "HIGH_FREQUENCY_DETECTION",
				RuleName:      "高频交易检测",
				Result:        model.RiskEventResultAlert,
				Reason:        "1分钟内交易次数超过阈值",
			})

			s.sendAlert(ctx, &RiskAlertMessage{
				AlertID:     uuid.New().String(),
				Wallet:      wallet,
				AlertType:   "HIGH_FREQUENCY_TRADING",
				Severity:    "critical",
				Description: "高频交易检测: 1分钟内交易次数过多",
				Context: map[string]string{
					"market":       trade.Market,
					"trade_count":  string(rune(count.TradesInWindow)),
				},
				CreatedAt: now.UnixMilli(),
			})

			logger.Warn("high frequency trading detected",
				"wallet", wallet,
				"market", trade.Market,
				"trade_count", count.TradesInWindow)

			// 只触发一次
			count.TradesInWindow = 0
		}
	}
}

// recordEvent 记录风控事件
func (s *TradeMonitorService) recordEvent(ctx context.Context, params *RiskEventParams) {
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
		logger.Error("failed to create trade monitor event", "error", err)
	}
}

// sendAlert 发送告警
func (s *TradeMonitorService) sendAlert(ctx context.Context, alert *RiskAlertMessage) {
	if s.onRiskAlert != nil {
		if err := s.onRiskAlert(ctx, alert); err != nil {
			logger.Error("failed to send trade monitor alert",
				"alert_id", alert.AlertID,
				"error", err)
		}
	}
}
