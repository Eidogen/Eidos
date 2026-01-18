package event

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	"github.com/eidos-exchange/eidos/eidos-market/internal/model"
	"github.com/eidos-exchange/eidos/eidos-market/internal/service"
)

// TradeProcessor 成交处理接口 (用于依赖注入和测试)
type TradeProcessor interface {
	ProcessTrade(ctx context.Context, trade *model.TradeEvent) error
}

// TradeHandler 成交事件处理器
// 消费 Kafka trade-results Topic
type TradeHandler struct {
	svc    TradeProcessor
	logger *zap.Logger
}

// NewTradeHandler 创建成交事件处理器
func NewTradeHandler(svc *service.MarketService, logger *zap.Logger) *TradeHandler {
	return &TradeHandler{
		svc:    svc,
		logger: logger.Named("trade_handler"),
	}
}

// NewTradeHandlerWithProcessor 创建成交事件处理器 (用于测试)
func NewTradeHandlerWithProcessor(svc TradeProcessor, logger *zap.Logger) *TradeHandler {
	return &TradeHandler{
		svc:    svc,
		logger: logger.Named("trade_handler"),
	}
}

// Handle 处理成交事件
// TODO [eidos-matching]: 需要 eidos-matching 发送成交结果到 Kafka
//
//	Topic: trade-results
//	Key: market 字段 (用于分区，如 "BTC-USDC")
//	Value: JSON 格式的 TradeEvent
//
//	消息格式 (model.TradeEvent):
//	{
//	  "trade_id": "uuid",
//	  "market": "BTC-USDC",
//	  "price": "50000.00",
//	  "amount": "1.5",
//	  "quote_amount": "75000.00",
//	  "taker_side": "buy",
//	  "maker_order_id": "uuid",
//	  "taker_order_id": "uuid",
//	  "timestamp": 1700000000000
//	}
func (h *TradeHandler) Handle(ctx context.Context, key, value []byte) error {
	var trade model.TradeEvent
	if err := json.Unmarshal(value, &trade); err != nil {
		h.logger.Error("failed to unmarshal trade event",
			zap.Error(err),
			zap.ByteString("value", value))
		return err
	}

	h.logger.Debug("received trade event",
		zap.String("trade_id", trade.TradeID),
		zap.String("market", trade.Market),
		zap.String("price", trade.Price),
		zap.String("amount", trade.Amount))

	if err := h.svc.ProcessTrade(ctx, &trade); err != nil {
		h.logger.Error("failed to process trade",
			zap.String("trade_id", trade.TradeID),
			zap.Error(err))
		return err
	}

	return nil
}

// Topic 返回订阅的 Topic
func (h *TradeHandler) Topic() string {
	return "trade-results"
}
