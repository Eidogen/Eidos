// Package engine 撮合引擎核心实现
package engine

import (
	"fmt"
	"time"

	"github.com/eidos-exchange/eidos/eidos-matching/internal/model"
	"github.com/eidos-exchange/eidos/eidos-matching/internal/orderbook"
	"github.com/shopspring/decimal"
)

// MatchResult 撮合结果
type MatchResult struct {
	Trades         []*model.TradeResult     // 成交列表
	OrderUpdates   []*model.OrderBookUpdate // 订单簿更新
	TakerOrder     *model.Order             // Taker 订单 (可能有剩余)
	TakerFilled    bool                     // Taker 是否完全成交
	TakerCancelled bool                     // Taker 是否被取消 (IOC/FOK 未完全成交)
}

// MarketConfig 市场配置
type MarketConfig struct {
	Symbol        string          // 交易对 (如 BTC-USDC)
	BaseToken     string          // 基础货币 (如 BTC)
	QuoteToken    string          // 计价货币 (如 USDC)
	PriceDecimals int             // 价格精度
	SizeDecimals  int             // 数量精度
	MinSize       decimal.Decimal // 最小下单数量
	TickSize      decimal.Decimal // 最小价格变动
	MakerFeeRate  decimal.Decimal // Maker 费率
	TakerFeeRate  decimal.Decimal // Taker 费率
	MaxSlippage   decimal.Decimal // 最大滑点 (市价单)
}

// Matcher 撮合器
// 实现价格优先、时间优先的撮合算法
type Matcher struct {
	config    *MarketConfig
	orderBook *orderbook.OrderBook

	// 最新价维护
	lastPrice    decimal.Decimal // 最新成交价
	lastPriceAt  int64           // 最新成交价时间
	indexPrice   decimal.Decimal // 外部指数价格 (用于市价单保护)
	indexPriceAt int64           // 指数价格更新时间

	// ID 生成
	tradeIDGen func() string

	// 统计
	totalTrades int64
	totalVolume decimal.Decimal
}

// NewMatcher 创建撮合器
func NewMatcher(config *MarketConfig, ob *orderbook.OrderBook, tradeIDGen func() string) *Matcher {
	return &Matcher{
		config:      config,
		orderBook:   ob,
		lastPrice:   decimal.Zero,
		indexPrice:  decimal.Zero,
		totalVolume: decimal.Zero,
		tradeIDGen:  tradeIDGen,
	}
}

// Match 执行撮合
// 根据订单类型分发到对应的撮合逻辑
func (m *Matcher) Match(order *model.Order) (*MatchResult, error) {
	switch order.Type {
	case model.OrderTypeLimit:
		return m.matchLimitOrder(order)
	case model.OrderTypeMarket:
		return m.matchMarketOrder(order)
	default:
		return nil, fmt.Errorf("unsupported order type: %d", order.Type)
	}
}

// matchLimitOrder 撮合限价单
func (m *Matcher) matchLimitOrder(order *model.Order) (*MatchResult, error) {
	result := &MatchResult{
		Trades:       make([]*model.TradeResult, 0),
		OrderUpdates: make([]*model.OrderBookUpdate, 0),
		TakerOrder:   order,
	}

	// FOK 订单预检查
	if order.TimeInForce == model.TimeInForceFOK {
		if !m.canFillCompletely(order) {
			result.TakerCancelled = true
			return result, nil
		}
	}

	// 执行撮合
	m.executeMatch(order, result)

	// 处理剩余数量
	if !order.IsFilled() {
		switch order.TimeInForce {
		case model.TimeInForceGTC:
			// GTC: 剩余部分加入订单簿
			if m.orderBook.AddOrder(order) {
				result.OrderUpdates = append(result.OrderUpdates, &model.OrderBookUpdate{
					Market:     order.Market,
					UpdateType: "ADD",
					Side:       order.Side,
					Price:      order.Price,
					Size:       order.Remaining,
					Timestamp:  time.Now().UnixNano(),
					Sequence:   m.orderBook.NextSequence(),
				})
			}
		case model.TimeInForceIOC, model.TimeInForceFOK:
			// IOC/FOK: 剩余部分取消
			result.TakerCancelled = true
		}
	}

	result.TakerFilled = order.IsFilled()
	return result, nil
}

// matchMarketOrder 撮合市价单
func (m *Matcher) matchMarketOrder(order *model.Order) (*MatchResult, error) {
	result := &MatchResult{
		Trades:       make([]*model.TradeResult, 0),
		OrderUpdates: make([]*model.OrderBookUpdate, 0),
		TakerOrder:   order,
	}

	// 获取参考价格 (用于滑点保护)
	refPrice := m.getReferencePrice()
	if refPrice.IsZero() {
		// 无参考价格，拒绝市价单
		result.TakerCancelled = true
		return result, fmt.Errorf("no reference price available for market order")
	}

	// 检查流动性
	var oppositeSide *orderbook.RBTree
	if order.Side == model.OrderSideBuy {
		oppositeSide = m.orderBook.Asks
	} else {
		oppositeSide = m.orderBook.Bids
	}

	if oppositeSide.Empty() {
		result.TakerCancelled = true
		return result, fmt.Errorf("no liquidity for market order")
	}

	// 计算滑点保护价格
	protectPrice := m.calculateProtectPrice(order.Side, refPrice)

	// 设置市价单的保护价格
	order.Price = protectPrice

	// 执行撮合
	m.executeMatch(order, result)

	// 市价单剩余部分自动取消 (IOC 语义)
	if !order.IsFilled() {
		result.TakerCancelled = true
	}

	result.TakerFilled = order.IsFilled()
	return result, nil
}

// executeMatch 执行撮合核心逻辑
// 价格优先、时间优先
func (m *Matcher) executeMatch(takerOrder *model.Order, result *MatchResult) {
	now := time.Now().UnixNano()

	for !takerOrder.IsFilled() {
		// 获取对手盘最优价格档位
		var bestLevel *model.PriceLevel
		if takerOrder.Side == model.OrderSideBuy {
			bestLevel = m.orderBook.BestAsk()
		} else {
			bestLevel = m.orderBook.BestBid()
		}

		// 对手盘为空，退出
		if bestLevel == nil {
			break
		}

		// 检查价格是否能成交
		if !m.canMatchPrice(takerOrder, bestLevel.Price) {
			break
		}

		// 遍历该价格档位的所有订单 (时间优先)
		makerOrder := bestLevel.FirstOrder()
		for makerOrder != nil && !takerOrder.IsFilled() {
			// 计算成交数量
			matchQty := decimal.Min(takerOrder.Remaining, makerOrder.Remaining)

			// 生成成交记录
			trade := m.createTrade(takerOrder, makerOrder, matchQty, now)
			result.Trades = append(result.Trades, trade)

			// 更新订单数量
			m.orderBook.UpdateOrderRemaining(makerOrder, matchQty)
			takerOrder.Remaining = takerOrder.Remaining.Sub(matchQty)

			// 更新最新价
			m.updateLastPrice(trade.Price, now)

			// 获取下一个 Maker 订单
			nextMaker := makerOrder.Next

			// 如果 Maker 订单完全成交，从订单簿移除
			if makerOrder.IsFilled() {
				m.orderBook.RemoveOrder(makerOrder.OrderID)
				result.OrderUpdates = append(result.OrderUpdates, &model.OrderBookUpdate{
					Market:     makerOrder.Market,
					UpdateType: "REMOVE",
					Side:       makerOrder.Side,
					Price:      makerOrder.Price,
					Size:       decimal.Zero,
					Timestamp:  now,
					Sequence:   m.orderBook.NextSequence(),
				})
			} else {
				// 部分成交，更新订单簿
				result.OrderUpdates = append(result.OrderUpdates, &model.OrderBookUpdate{
					Market:     makerOrder.Market,
					UpdateType: "UPDATE",
					Side:       makerOrder.Side,
					Price:      makerOrder.Price,
					Size:       makerOrder.Remaining,
					Timestamp:  now,
					Sequence:   m.orderBook.NextSequence(),
				})
			}

			makerOrder = nextMaker
		}

		// 如果价格档位为空，会在下次循环时自动跳过
	}

	// 更新统计
	for _, trade := range result.Trades {
		m.totalTrades++
		m.totalVolume = m.totalVolume.Add(trade.Amount)
	}
}

// canMatchPrice 检查价格是否能成交
func (m *Matcher) canMatchPrice(takerOrder *model.Order, makerPrice decimal.Decimal) bool {
	if takerOrder.Side == model.OrderSideBuy {
		// 买单: Taker 价格 >= Maker 价格
		return takerOrder.Price.GreaterThanOrEqual(makerPrice)
	}
	// 卖单: Taker 价格 <= Maker 价格
	return takerOrder.Price.LessThanOrEqual(makerPrice)
}

// canFillCompletely FOK 订单预检查
// 检查订单簿是否有足够流动性完全成交
func (m *Matcher) canFillCompletely(order *model.Order) bool {
	remaining := order.Amount
	var tree *orderbook.RBTree

	if order.Side == model.OrderSideBuy {
		tree = m.orderBook.Asks
	} else {
		tree = m.orderBook.Bids
	}

	// 遍历对手盘
	canFill := false
	tree.InOrder(func(pl *model.PriceLevel) bool {
		// 检查价格
		if !m.canMatchPrice(order, pl.Price) {
			return false // 价格不匹配，停止遍历
		}

		// 累计可成交数量
		if pl.TotalSize.GreaterThanOrEqual(remaining) {
			remaining = decimal.Zero
			canFill = true
			return false // 足够成交，停止遍历
		}
		remaining = remaining.Sub(pl.TotalSize)
		return true // 继续遍历
	})

	return canFill && remaining.IsZero()
}

// createTrade 创建成交记录
func (m *Matcher) createTrade(
	takerOrder *model.Order,
	makerOrder *model.Order,
	matchQty decimal.Decimal,
	timestamp int64,
) *model.TradeResult {
	// 成交价格使用 Maker 价格
	tradePrice := makerOrder.Price
	quoteAmount := tradePrice.Mul(matchQty)

	// 计算手续费
	makerFee := quoteAmount.Mul(m.config.MakerFeeRate)
	takerFee := quoteAmount.Mul(m.config.TakerFeeRate)

	return &model.TradeResult{
		TradeID:          m.tradeIDGen(),
		Market:           takerOrder.Market,
		MakerOrderID:     makerOrder.OrderID,
		TakerOrderID:     takerOrder.OrderID,
		MakerWallet:      makerOrder.Wallet,
		TakerWallet:      takerOrder.Wallet,
		Price:            tradePrice,
		Amount:           matchQty,
		QuoteAmount:      quoteAmount,
		MakerSide:        makerOrder.Side,
		MakerFee:         makerFee,
		TakerFee:         takerFee,
		FeeToken:         m.config.QuoteToken, // 手续费使用计价货币
		MakerOrderFilled: makerOrder.Remaining.Sub(matchQty).IsZero(),
		TakerOrderFilled: takerOrder.Remaining.Sub(matchQty).IsZero(),
		Timestamp:        timestamp,
		Sequence:         m.orderBook.NextSequence(),
	}
}

// getReferencePrice 获取参考价格 (用于市价单滑点保护)
// 优先级: 最近成交价 > 中间价 > 指数价
func (m *Matcher) getReferencePrice() decimal.Decimal {
	now := time.Now().UnixNano()
	staleThreshold := int64(5 * time.Minute)
	indexStaleThreshold := int64(1 * time.Minute)

	// 1. 最近成交价 (5分钟内有效)
	if !m.lastPrice.IsZero() && (now-m.lastPriceAt) < staleThreshold {
		return m.lastPrice
	}

	// 2. 中间价
	bestBid := m.orderBook.BestBidPrice()
	bestAsk := m.orderBook.BestAskPrice()
	if !bestBid.IsZero() && !bestAsk.IsZero() {
		return bestBid.Add(bestAsk).Div(decimal.NewFromInt(2))
	}

	// 3. 外部指数价格 (1分钟内有效)
	if !m.indexPrice.IsZero() && (now-m.indexPriceAt) < indexStaleThreshold {
		return m.indexPrice
	}

	return decimal.Zero
}

// calculateProtectPrice 计算市价单保护价格
func (m *Matcher) calculateProtectPrice(side model.OrderSide, refPrice decimal.Decimal) decimal.Decimal {
	slippage := m.config.MaxSlippage
	if slippage.IsZero() {
		slippage = decimal.NewFromFloat(0.05) // 默认 5% 滑点
	}

	if side == model.OrderSideBuy {
		// 买单: 保护价格 = 参考价 * (1 + 滑点)
		return refPrice.Mul(decimal.NewFromInt(1).Add(slippage))
	}
	// 卖单: 保护价格 = 参考价 * (1 - 滑点)
	return refPrice.Mul(decimal.NewFromInt(1).Sub(slippage))
}

// updateLastPrice 更新最新成交价
func (m *Matcher) updateLastPrice(price decimal.Decimal, timestamp int64) {
	m.lastPrice = price
	m.lastPriceAt = timestamp
	m.orderBook.SetLastPrice(price, timestamp)
}

// UpdateIndexPrice 更新外部指数价格
// TODO: 由外部价格服务调用
func (m *Matcher) UpdateIndexPrice(price decimal.Decimal) {
	m.indexPrice = price
	m.indexPriceAt = time.Now().UnixNano()
}

// GetLastPrice 获取最新成交价
func (m *Matcher) GetLastPrice() decimal.Decimal {
	return m.lastPrice
}

// GetStats 获取撮合统计
func (m *Matcher) GetStats() MatcherStats {
	return MatcherStats{
		TotalTrades: m.totalTrades,
		TotalVolume: m.totalVolume,
		LastPrice:   m.lastPrice,
		LastPriceAt: m.lastPriceAt,
	}
}

// MatcherStats 撮合器统计
type MatcherStats struct {
	TotalTrades int64           `json:"total_trades"`
	TotalVolume decimal.Decimal `json:"total_volume"`
	LastPrice   decimal.Decimal `json:"last_price"`
	LastPriceAt int64           `json:"last_price_at"`
}
