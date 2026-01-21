// Package publisher 提供 Kafka 消息发布功能
package publisher

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/eidos-exchange/eidos/eidos-common/pkg/logger"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/kafka"
	"github.com/eidos-exchange/eidos/eidos-trading/internal/model"
	"github.com/shopspring/decimal"
)

// SettlementPublisher 结算消息发布者
// 发布消息到 settlements topic，供 eidos-chain 上链结算
type SettlementPublisher struct {
	producer KafkaProducer
}

// NewSettlementPublisher 创建结算发布者
func NewSettlementPublisher(producer KafkaProducer) *SettlementPublisher {
	return &SettlementPublisher{
		producer: producer,
	}
}

// SettlementTradeMessage 待结算的成交消息 (发送到 eidos-chain)
// 格式对应 eidos-chain/internal/model.SettlementTrade
type SettlementTradeMessage struct {
	TradeID      string `json:"trade_id"`
	Market       string `json:"market"`
	MakerWallet  string `json:"maker_wallet"`
	TakerWallet  string `json:"taker_wallet"`
	MakerOrderID string `json:"maker_order_id"`
	TakerOrderID string `json:"taker_order_id"`
	Price        string `json:"price"`
	Amount       string `json:"amount"`       // Base token 数量
	QuoteAmount  string `json:"quote_amount"` // Quote token 数量
	MakerFee     string `json:"maker_fee"`
	TakerFee     string `json:"taker_fee"`
	MakerSide    int8   `json:"maker_side"` // 1=buy, 2=sell
	MatchedAt    int64  `json:"matched_at"`
}

// PublishSettlementTrade 发布单笔成交到 settlements topic
func (p *SettlementPublisher) PublishSettlementTrade(ctx context.Context, trade *model.Trade) error {
	if p.producer == nil {
		return nil // Kafka 未启用
	}

	msg := &SettlementTradeMessage{
		TradeID:      trade.TradeID,
		Market:       trade.Market,
		MakerWallet:  trade.MakerWallet,
		TakerWallet:  trade.TakerWallet,
		MakerOrderID: trade.MakerOrderID,
		TakerOrderID: trade.TakerOrderID,
		Price:        trade.Price.String(),
		Amount:       trade.Amount.String(),
		QuoteAmount:  trade.QuoteAmount.String(),
		MakerFee:     trade.MakerFee.String(),
		TakerFee:     trade.TakerFee.String(),
		MakerSide:    int8(trade.MakerSide),
		MatchedAt:    trade.MatchedAt,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal settlement trade message: %w", err)
	}

	// 使用 TradeID 作为分区键，保证同一成交的消息顺序
	if err := p.producer.SendWithContext(ctx, kafka.TopicSettlements, []byte(trade.TradeID), data); err != nil {
		logger.Error("publish settlement trade failed",
			"trade_id", trade.TradeID,
			"error", err)
		return fmt.Errorf("send settlement trade: %w", err)
	}

	logger.Debug("settlement trade published",
		"trade_id", trade.TradeID,
		"market", trade.Market)

	return nil
}

// PublishSettlementTradeFromResult 从成交结果消息发布到 settlements topic
func (p *SettlementPublisher) PublishSettlementTradeFromResult(
	ctx context.Context,
	tradeID, market, makerWallet, takerWallet string,
	makerOrderID, takerOrderID string,
	price, amount, quoteAmount, makerFee, takerFee decimal.Decimal,
	makerIsBuyer bool,
	matchedAt int64,
) error {
	if p.producer == nil {
		return nil // Kafka 未启用
	}

	var makerSide int8 = 2 // sell
	if makerIsBuyer {
		makerSide = 1 // buy
	}

	msg := &SettlementTradeMessage{
		TradeID:      tradeID,
		Market:       market,
		MakerWallet:  makerWallet,
		TakerWallet:  takerWallet,
		MakerOrderID: makerOrderID,
		TakerOrderID: takerOrderID,
		Price:        price.String(),
		Amount:       amount.String(),
		QuoteAmount:  quoteAmount.String(),
		MakerFee:     makerFee.String(),
		TakerFee:     takerFee.String(),
		MakerSide:    makerSide,
		MatchedAt:    matchedAt,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal settlement trade message: %w", err)
	}

	if err := p.producer.SendWithContext(ctx, kafka.TopicSettlements, []byte(tradeID), data); err != nil {
		logger.Error("publish settlement trade failed",
			"trade_id", tradeID,
			"error", err)
		return fmt.Errorf("send settlement trade: %w", err)
	}

	logger.Debug("settlement trade published",
		"trade_id", tradeID,
		"market", market)

	return nil
}
